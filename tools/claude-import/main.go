// claude-import imports historical transcripts only. It never starts OMO or tools.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type receipt struct {
	ID, Name, Path, Status, SHA256 string
	Records                        int
}

func main() {
	root := flag.String("root", "", "Claude-Cowork root (required)")
	sessions := flag.String("sessions", "", "OMO sessions root (required)")
	dry := flag.Bool("dry-run", false, "scan and convert without writing")
	flag.Parse()
	if err := run(*root, *sessions, *dry); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root, sessions string, dry bool) error {
	if !filepath.IsAbs(root) || !filepath.IsAbs(sessions) {
		return errors.New("-root and -sessions must be absolute paths")
	}
	sources, scanned, err := discover(filepath.Clean(root))
	if err != nil {
		return err
	}
	receipts := make([]receipt, 0, len(sources))
	for _, s := range sources {
		data, err := convert(s, sessions)
		if err != nil {
			return err
		}
		path, status := outputPath(s, sessions), "dry-run"
		if !dry {
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			// O_EXCL is deliberate: never overwrite a session already continued in OMO.
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			switch {
			case errors.Is(err, os.ErrExist):
				status = "skipped"
			case err != nil:
				return err
			default:
				_, writeErr := f.Write(data)
				closeErr := f.Close()
				if err := errors.Join(writeErr, closeErr); err != nil {
					removeErr := os.Remove(path)
					return errors.Join(err, removeErr)
				}
				status = "imported"
			}
		}
		receipts = append(receipts, receipt{s.ID, s.Name, path, status, s.Hash, len(s.Records)})
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Scanned  int       `json:"scanned"`
		Sessions []receipt `json:"sessions"`
	}{scanned, receipts})
}
