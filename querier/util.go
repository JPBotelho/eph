package querier

import (
	"bytes"
	"io"
)

func CleanupScrapeFile(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Normalize Windows CRLF → LF
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))

	// Filter out comment lines
	var cleaned []byte
	for _, line := range bytes.Split(b, []byte("\n")) {
		if len(line) > 0 && line[0] == '#' {
			continue
		}
		cleaned = append(cleaned, line...)
		cleaned = append(cleaned, '\n')
	}

	return cleaned, nil
}

func CleanupScrapeBytes(data []byte) ([]byte, error) {

	// Normalize Windows CRLF → LF
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

	// Filter out comment lines
	var cleaned []byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) > 0 && line[0] == '#' {
			continue
		}
		cleaned = append(cleaned, line...)
		cleaned = append(cleaned, '\n')
	}

	return cleaned, nil
}
