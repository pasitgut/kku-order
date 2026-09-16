package main

import (
	"errors"
	"io"
	"strings"
)

func validateFileSignature(file io.ReadSeeker, extension string) error {
	extension = strings.ToLower(extension)
	if extension == ".csv" {
		return nil
	}
	buffer := make([]byte, 8)
	read, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	switch extension {
	case ".pdf":
		if read < 5 || string(buffer[:5]) != "%PDF-" {
			return errors.New("ไฟล์ PDF มีรูปแบบไม่ถูกต้อง")
		}
	case ".docx", ".xlsx":
		if read < 4 || string(buffer[:4]) != "PK\x03\x04" {
			return errors.New("ไฟล์เอกสารมีรูปแบบไม่ถูกต้อง")
		}
	}
	return nil
}
