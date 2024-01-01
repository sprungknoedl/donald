package main

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Upload performs an SFTP upload using the provided configuration.
func Upload(cfg Configuration) error {
	// Establish an SSH connection to the SFTP server
	conn, err := ssh.Dial("tcp", cfg.SftpAddr, &ssh.ClientConfig{
		User:            cfg.SftpUser,
		Auth:            []ssh.AuthMethod{ssh.Password(cfg.SftpPass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Ignore host key verification (insecure)
	})
	if err != nil {
		return err
	}

	// Create an SFTP client using the SSH connection
	client, err := sftp.NewClient(conn)
	if err != nil {
		return err
	}

	// Open the target file on the SFTP server for writing
	dst, err := client.OpenFile(filepath.Join(cfg.SftpDir, cfg.SftpFile), os.O_WRONLY|os.O_TRUNC|os.O_CREATE)
	if err != nil {
		return err
	}

	// Open the local file for reading
	src, err := os.Open(filepath.Join(cfg.OutputDir, cfg.OutputFile))
	if err != nil {
		return err
	}

	// Copy the content of the local file to the SFTP server
	_, err = io.Copy(dst, src)
	return err
}
