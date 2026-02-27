package cmd

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/georgele/devctl/internal/vault"
	"github.com/spf13/cobra"
)

func BackupCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Back up the vault to a compressed archive",
		Long:  "Create a compressed, encrypted backup of the entire vault directory.",
		Example: `  envsafe backup
  envsafe backup -o my-backup.tar.gz`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			if !vault.Exists(projectRoot) {
				return fmt.Errorf("no vault found. Run 'envsafe init' first")
			}

			if output == "" {
				output = fmt.Sprintf("envsafe-backup-%s.tar.gz", time.Now().Format("20060102-150405"))
			}

			vaultDir := vault.VaultPath(projectRoot)

			f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				return fmt.Errorf("creating backup file: %w", err)
			}
			defer f.Close()

			gw := gzip.NewWriter(f)
			defer gw.Close()

			tw := tar.NewWriter(gw)
			defer tw.Close()

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
				// Clean up partial file on error
				os.Remove(output)
				return fmt.Errorf("creating backup: %w", err)
			}

			fmt.Printf("Vault backed up to %s\n", output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: envsafe-backup-<timestamp>.tar.gz)")

	return cmd
}

func RestoreCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "restore <archive>",
		Short: "Restore a vault from a backup archive",
		Args:  cobra.ExactArgs(1),
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

			f, err := os.Open(archive)
			if err != nil {
				return fmt.Errorf("opening archive: %w", err)
			}
			defer f.Close()

			gr, err := gzip.NewReader(f)
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
