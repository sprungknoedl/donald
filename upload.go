package main

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func Upload(cfg *Configuration) error {
	conn, err := ssh.Dial("tcp", cfg.SftpAddr, &ssh.ClientConfig{
		User:            cfg.SftpUser,
		Auth:            []ssh.AuthMethod{ssh.Password(cfg.SftpPass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return err
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		return err
	}

	dst, err := client.OpenFile(filepath.Join(cfg.SftpDir, cfg.SftpFile), os.O_WRONLY|os.O_TRUNC|os.O_CREATE)
	if err != nil {
		return err
	}

	src, err := os.Open(filepath.Join(cfg.OutputDir, cfg.OutputFile))
	if err != nil {
		ErrrLogger.Fatalf("Error opening archive for upload: %v (%T)", err, err)
	}

	_, err = io.Copy(dst, src)
	return err
}
