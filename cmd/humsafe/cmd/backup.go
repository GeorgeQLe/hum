package cmd

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/georgele/hum/internal/vault"
	"github.com/georgele/hum/internal/vault/crypto"
	"github.com/spf13/cobra"
)

// backupMagic is prepended to encrypted backups to identify them.
var backupMagic = []byte("HUMSAFE_ENC\x01")

func BackupCmd() *cobra.Command {
	var output string
	var noEncrypt bool

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Back up the vault to a compressed, encrypted archive",
		Long: `Create a compressed, encrypted backup of the entire vault directory.

By default the backup is encrypted with AES-256-GCM using a key derived from
the vault password. Use --no-encrypt for an unencrypted archive (the vault.enc
file inside is still encrypted, but config.json and audit.log will be plaintext).`,
		Example: `  humsafe backup
  humsafe backup -o my-backup.tar.gz
  humsafe backup --no-encrypt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			if !vault.Exists(projectRoot) {
				return fmt.Errorf("no vault found. Run 'humsafe init' first")
			}

			ext := ".tar.gz.enc"
			if noEncrypt {
				ext = ".tar.gz"
			}
			if output == "" {
				output = fmt.Sprintf("humsafe-backup-%s%s", time.Now().Format("20060102-150405"), ext)
			}

			vaultDir := vault.VaultPath(projectRoot)

			// Create tar.gz in memory buffer
			var tarBuf bytes.Buffer
			gw := gzip.NewWriter(&tarBuf)
			tw := tar.NewWriter(gw)

			err = filepath.Walk(vaultDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				relPath, err := filepath.Rel(projectRoot, path)
				if err != nil {
					return err
				}

				header, err := tar.FileInfoHeader(info, "")
				if err != nil {
					return err
				}
				header.Name = relPath

				if err := tw.WriteHeader(header); err != nil {
					return err
				}

				if info.IsDir() {
					return nil
				}

				file, err := os.Open(path)
				if err != nil {
					return err
				}
				defer file.Close()

				_, err = io.Copy(tw, file)
				return err
			})
			if err != nil {
				return fmt.Errorf("creating backup: %w", err)
			}

			if err := tw.Close(); err != nil {
				return fmt.Errorf("finalizing tar: %w", err)
			}
			if err := gw.Close(); err != nil {
				return fmt.Errorf("finalizing gzip: %w", err)
			}

			var outputData []byte
			if noEncrypt {
				outputData = tarBuf.Bytes()
			} else {
				// Prompt for password and derive encryption key
				password, err := promptPassword("Enter vault password for backup encryption: ")
				if err != nil {
					return err
				}

				salt, err := crypto.GenerateSalt()
				if err != nil {
					return fmt.Errorf("generating salt: %w", err)
				}

				key, err := crypto.DeriveKey(password, salt)
				if err != nil {
					return fmt.Errorf("deriving key: %w", err)
				}

				encrypted, err := crypto.Encrypt(key, tarBuf.Bytes())
				if err != nil {
					return fmt.Errorf("encrypting backup: %w", err)
				}

				// Format: magic + salt + encrypted(tar.gz)
				outputData = make([]byte, 0, len(backupMagic)+len(salt)+len(encrypted))
				outputData = append(outputData, backupMagic...)
				outputData = append(outputData, salt...)
				outputData = append(outputData, encrypted...)
			}

			if err := os.WriteFile(output, outputData, 0600); err != nil {
				return fmt.Errorf("writing backup file: %w", err)
			}

			if noEncrypt {
				fmt.Printf("Vault backed up to %s (unencrypted)\n", output)
			} else {
				fmt.Printf("Vault backed up to %s (encrypted)\n", output)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: humsafe-backup-<timestamp>.tar.gz.enc)")
	cmd.Flags().BoolVar(&noEncrypt, "no-encrypt", false, "Create unencrypted backup (tar.gz only)")

	return cmd
}

func RestoreCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "restore <archive>",
		Short: "Restore a vault from a backup archive",
		Long: `Restore a vault from a backup archive. Supports both encrypted (.tar.gz.enc)
and unencrypted (.tar.gz) archives. Encrypted archives will prompt for the
password used during backup.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			archive := args[0]

			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			vaultDir := vault.VaultPath(projectRoot)

			if vault.Exists(projectRoot) && !force {
				fmt.Fprintf(os.Stderr, "A vault already exists at %s. Overwrite? [y/N] ", vaultDir)
				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(answer)) != "y" {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			rawData, err := os.ReadFile(archive)
			if err != nil {
				return fmt.Errorf("reading archive: %w", err)
			}

			var tarGzData []byte
			if bytes.HasPrefix(rawData, backupMagic) {
				// Encrypted backup: magic + salt(16) + encrypted data
				data := rawData[len(backupMagic):]
				if len(data) < crypto.SaltLen {
					return fmt.Errorf("encrypted backup is too short")
				}
				salt := data[:crypto.SaltLen]
				ciphertext := data[crypto.SaltLen:]

				password, err := promptPassword("Enter backup password: ")
				if err != nil {
					return err
				}

				key, err := crypto.DeriveKey(password, salt)
				if err != nil {
					return fmt.Errorf("deriving key: %w", err)
				}

				tarGzData, err = crypto.Decrypt(key, ciphertext)
				if err != nil {
					return fmt.Errorf("decryption failed (wrong password or corrupted backup)")
				}
			} else {
				// Unencrypted backup
				tarGzData = rawData
			}

			gr, err := gzip.NewReader(bytes.NewReader(tarGzData))
			if err != nil {
				return fmt.Errorf("reading archive: %w", err)
			}
			defer gr.Close()

			tr := tar.NewReader(gr)

			for {
				header, err := tr.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					return fmt.Errorf("reading archive entry: %w", err)
				}

				// Prevent path traversal
				target := filepath.Join(projectRoot, header.Name)
				if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(projectRoot)+string(os.PathSeparator)) {
					return fmt.Errorf("archive contains invalid path: %s", header.Name)
				}

				switch header.Typeflag {
				case tar.TypeDir:
					if err := os.MkdirAll(target, 0700); err != nil {
						return fmt.Errorf("creating directory: %w", err)
					}
				case tar.TypeReg:
					if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
						return fmt.Errorf("creating parent directory: %w", err)
					}
					outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
					if err != nil {
						return fmt.Errorf("creating file: %w", err)
					}
					if _, err := io.Copy(outFile, io.LimitReader(tr, 100*1024*1024)); err != nil {
						outFile.Close()
						return fmt.Errorf("writing file: %w", err)
					}
					outFile.Close()
				}
			}

			fmt.Printf("Vault restored from %s\n", archive)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}
