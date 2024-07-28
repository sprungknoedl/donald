package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// UploadSFTP performs an SFTP upload using the provided configuration.
func UploadSFTP(cfg Configuration) error {
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
	defer src.Close()

	// Copy the content of the local file to the SFTP server
	_, err = io.Copy(dst, src)
	return err
}

// UploadDagobert performs an upload to a Dagobert instance using the provided configuration.
func UploadDagobert(cfg Configuration) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// don't fail if hostname can't be resolved, just use empty string instead
	hostname, _ := os.Hostname()

	// Set multipart fields
	writer.WriteField("Type", "Triage")
	writer.WriteField("Name", cfg.DagobertFile)
	writer.WriteField("Source", hostname)
	part, _ := writer.CreateFormFile("File", cfg.DagobertFile)

	// Open the local file for reading
	src, err := os.Open(filepath.Join(cfg.OutputDir, cfg.OutputFile))
	if err != nil {
		return err
	}
	defer src.Close()

	// Copy the content of the local file to the multipart message
	_, err = io.Copy(part, src)
	if err != nil {
		return err
	}

	// Finish the multipart message
	err = writer.Close()
	if err != nil {
		return err
	}

	dst, err := url.JoinPath(cfg.DagobertAddr, "/cases/", cfg.DagobertCase, "/evidences/")
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", dst, body)
	if err != nil {
		return err
	}

	req.Header.Set("X-API-Key", cfg.DagobertKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	_, err = http.DefaultClient.Do(req)
	return err
}
