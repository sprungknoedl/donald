package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// UploadSFTP performs an SFTP upload using the provided configuration. sum is
// the SHA-256 hex digest of the archive, uploaded as a sidecar alongside it.
func UploadSFTP(cfg Configuration, sum string) error {
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
	if _, err = io.Copy(dst, src); err != nil {
		return err
	}

	// Upload the hash sidecar alongside the archive. Its body is regenerated
	// from the digest so the embedded filename matches the remote archive name
	// (which may differ from the local one via -sftp-file). A failure here is
	// logged but does not fail the run.
	if sum != "" {
		body := fmt.Sprintf("%s  %s\n", sum, filepath.Base(cfg.SftpFile))
		sdst, err := client.OpenFile(filepath.Join(cfg.SftpDir, cfg.SftpFile+".sha256"), os.O_WRONLY|os.O_TRUNC|os.O_CREATE)
		if err != nil {
			WarnLogger.Printf("Stage 3 (SFTP): Failed to upload hash sidecar: %v", err)
			return nil
		}
		if _, err := sdst.Write([]byte(body)); err != nil {
			WarnLogger.Printf("Stage 3 (SFTP): Failed to upload hash sidecar: %v", err)
		}
		sdst.Close()
	}

	return nil
}

// UploadDagobert performs an upload to a Dagobert instance using the provided
// configuration. sum is the SHA-256 hex digest of the archive, sent as a Hash
// field so the server can verify the upload.
func UploadDagobert(cfg Configuration, sum string) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// don't fail if hostname can't be resolved, just use empty string instead
	hostname, _ := os.Hostname()

	// Set multipart fields
	writer.WriteField("Type", "Triage")
	writer.WriteField("Name", cfg.DagobertFile)
	writer.WriteField("Source", hostname)
	writer.WriteField("Hash", sum)
	if cfg.ZipPass != "" {
		writer.WriteField("Password", cfg.ZipPass)
	}
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

	dst, err := url.JoinPath(cfg.DagobertAddr, "/cases/", cfg.DagobertCase, "/evidences/new")
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
