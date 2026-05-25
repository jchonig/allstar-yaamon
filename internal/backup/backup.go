// Package backup implements .owbackup file creation, inspection, and restore.
//
// File format:
//
//	[4]  magic "YAAM"
//	[1]  format version (0x01)
//	[1]  flags: 0x01 = encrypted
//	[4]  manifest length (big-endian uint32)
//	[N]  manifest JSON (always plaintext — readable without passphrase)
//	[…]  body: plaintext tar.gz  OR  salt(16)+nonce(12)+ciphertext of tar.gz
package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"allstar-yaamon/internal/db"
)

const (
	fileMagic         = "YAAM"
	fileFormatVersion = 0x01
	flagEncrypted     = 0x01
)

// Manifest describes the contents and metadata of a .owbackup file.
type Manifest struct {
	Format        string    `json:"format"`
	FormatVersion int       `json:"format_version"`
	AppVersion    string    `json:"app_version"`
	SchemaVersion int       `json:"schema_version"`
	CreatedAt     time.Time `json:"created_at"`
	Hostname      string    `json:"hostname"`
	Encrypted     bool      `json:"encrypted"`
	Contents      struct {
		Nodes     int `json:"nodes"`
		Favorites int `json:"favorites"`
		Users     int `json:"users"`
		Configs   int `json:"configs"`
	} `json:"contents"`
}

// CreateOptions controls backup creation.
type CreateOptions struct {
	Passphrase string // empty = no encryption
}

// Create produces a .owbackup file as a byte slice.
// It snapshots the database online (no service stop required) then packages
// the snapshot into a gzip'd tar archive with an embedded manifest.
func Create(ctx context.Context, database *db.DB, appVersion string, opts CreateOptions) ([]byte, *Manifest, error) {
	// Gather metadata.
	schema, err := database.SchemaVersion(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("schema version: %w", err)
	}
	stats, err := database.Stats(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("db stats: %w", err)
	}
	hostname, _ := os.Hostname()

	manifest := &Manifest{
		Format:        "owbackup",
		FormatVersion: 1,
		AppVersion:    appVersion,
		SchemaVersion: schema,
		CreatedAt:     time.Now().UTC(),
		Hostname:      hostname,
		Encrypted:     opts.Passphrase != "",
	}
	manifest.Contents.Nodes = stats.Nodes
	manifest.Contents.Favorites = stats.Favorites
	manifest.Contents.Users = stats.Users
	manifest.Contents.Configs = stats.Configs

	// Snapshot the live database to a temp file.
	tmp, err := os.CreateTemp("", "yaamon-snap-*.db")
	if err != nil {
		return nil, nil, fmt.Errorf("temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	if err := database.Snapshot(ctx, tmpPath); err != nil {
		return nil, nil, fmt.Errorf("snapshot: %w", err)
	}

	// Build tar.gz containing manifest.json + yaamon.db.
	tarGz, err := buildTarGz(tmpPath, manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("archive: %w", err)
	}

	// Optionally encrypt the tar.gz body.
	var body []byte
	if opts.Passphrase != "" {
		body, err = encrypt(tarGz, opts.Passphrase)
		if err != nil {
			return nil, nil, fmt.Errorf("encrypt: %w", err)
		}
	} else {
		body = tarGz
	}

	// Assemble the final file: header + plaintext manifest + body.
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, err
	}

	var buf bytes.Buffer
	buf.WriteString(fileMagic)
	buf.WriteByte(fileFormatVersion)
	var flags byte
	if opts.Passphrase != "" {
		flags |= flagEncrypted
	}
	buf.WriteByte(flags)
	mlen := make([]byte, 4)
	binary.BigEndian.PutUint32(mlen, uint32(len(manifestJSON)))
	buf.Write(mlen)
	buf.Write(manifestJSON)
	buf.Write(body)

	return buf.Bytes(), manifest, nil
}

// Inspect reads the manifest from a .owbackup byte slice without decrypting.
func Inspect(data []byte) (*Manifest, error) {
	_, manifest, _, err := splitFile(data)
	return manifest, err
}

// RestoreOptions controls restore behaviour.
type RestoreOptions struct {
	Passphrase string
}

// Restore replaces the live database with the contents of a .owbackup file.
// It first creates a plaintext pre-restore backup next to the database file.
// Returns the path of the pre-restore backup on success.
func Restore(ctx context.Context, database *db.DB, appVersion string, data []byte, opts RestoreOptions) (preRestorePath string, err error) {
	flags, manifest, body, err := splitFile(data)
	if err != nil {
		return "", err
	}
	_ = manifest

	// Decrypt body if needed.
	if flags&flagEncrypted != 0 {
		if opts.Passphrase == "" {
			return "", fmt.Errorf("backup is encrypted — passphrase required")
		}
		body, err = decrypt(body, opts.Passphrase)
		if err != nil {
			return "", err
		}
	}

	// Extract yaamon.db from the tar.gz body.
	dbBytes, err := extractDBFromTarGz(body)
	if err != nil {
		return "", fmt.Errorf("extract archive: %w", err)
	}

	dbPath := database.Path()
	dir := filepath.Dir(dbPath)

	// Create a pre-restore backup (plaintext, no passphrase needed for safety net).
	preRestorePath = filepath.Join(dir, fmt.Sprintf("pre-restore-%s.owbackup",
		time.Now().UTC().Format("20060102T150405Z")))
	preData, _, err := Create(ctx, database, appVersion, CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("pre-restore backup: %w", err)
	}
	if err := os.WriteFile(preRestorePath, preData, 0600); err != nil {
		return "", fmt.Errorf("write pre-restore backup: %w", err)
	}

	// Write the restored DB to a temp file, then atomically replace the live DB.
	tmpRestore, err := os.CreateTemp(dir, "yaamon-restore-*.db")
	if err != nil {
		return preRestorePath, fmt.Errorf("temp restore file: %w", err)
	}
	restorePath := tmpRestore.Name()
	if _, err := tmpRestore.Write(dbBytes); err != nil {
		tmpRestore.Close()
		os.Remove(restorePath)
		return preRestorePath, fmt.Errorf("write restore file: %w", err)
	}
	tmpRestore.Close()

	if err := os.Rename(restorePath, dbPath); err != nil {
		os.Remove(restorePath)
		return preRestorePath, fmt.Errorf("replace database: %w", err)
	}

	return preRestorePath, nil
}

// splitFile parses the file header and returns (flags, manifest, body, error).
func splitFile(data []byte) (flags byte, manifest *Manifest, body []byte, err error) {
	if len(data) < 4+1+1+4 {
		return 0, nil, nil, fmt.Errorf("not a valid .owbackup file (too short)")
	}
	if string(data[:4]) != fileMagic {
		return 0, nil, nil, fmt.Errorf("not a valid .owbackup file (bad magic)")
	}
	// data[4] = format version (ignored for forward compat)
	flags = data[5]
	mlen := binary.BigEndian.Uint32(data[6:10])
	if int(10+mlen) > len(data) {
		return 0, nil, nil, fmt.Errorf("corrupt .owbackup file (manifest length exceeds file)")
	}
	manifest = &Manifest{}
	if err = json.Unmarshal(data[10:10+mlen], manifest); err != nil {
		return 0, nil, nil, fmt.Errorf("corrupt manifest: %w", err)
	}
	body = data[10+mlen:]
	return flags, manifest, body, nil
}

// buildTarGz creates a gzip'd tar containing manifest.json and yaamon.db.
func buildTarGz(dbPath string, manifest *Manifest) ([]byte, error) {
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	dbData, err := os.ReadFile(dbPath)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for _, f := range []struct {
		name string
		data []byte
	}{
		{"manifest.json", manifestJSON},
		{"yaamon.db", dbData},
	} {
		if err := tw.WriteHeader(&tar.Header{
			Name: f.name,
			Mode: 0600,
			Size: int64(len(f.data)),
		}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(f.data); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// extractDBFromTarGz reads yaamon.db from a gzip'd tar archive.
func extractDBFromTarGz(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == "yaamon.db" {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("yaamon.db not found in archive")
}
