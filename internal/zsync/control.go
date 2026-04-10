package zsync

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func ParseControlFile(zsyncControlFile io.Reader) (cf *ControlFile, err error) {
	cf = &ControlFile{}
	reader := bufio.NewReader(zsyncControlFile)

	err = cf.parseControlHeader(reader)
	if err != nil {
		return cf, err
	}
	err = cf.parseChecksums(reader)

	return cf, nil
}

func (cf *ControlFile) parseControlHeader(reader *bufio.Reader) error {
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := scanner.Text()

		if line == "\n" {
			break
		}

		key, val := readHeaderLine(line)
		setHeader(cf, key, val)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func readHeaderLine(line string) (key string, val string) {
	parts := strings.SplitN(line, ":", 2)
	key = strings.ToLower(parts[0])
	if len(parts) == 2 {
		val = strings.TrimSpace(parts[1])
	}

	return key, val
}

func setHeader(cf *ControlFile, key string, val string) error {
	switch key {
	case "filename":
		cf.Filename = val
	case "blocksize":
		blockSize, err := strconv.Atoi(val)
		if err != nil {
			return err
		}
		cf.BlockSize = uint32(blockSize)
	case "length":
		length, err := strconv.Atoi(val)
		if err != nil {
			return err
		}
		cf.Length = int64(length)
	case "hash-lengths":
		hashLengths := strings.Split(val, ",")
		if len(hashLengths) != 3 {
			return fmt.Errorf("invalid hash-lengths format")
		}
		hls := make([]uint8, 0, len(hashLengths))
		for _, hl := range hashLengths {
			hli, err := strconv.ParseUint(hl, 10, 0)
			if err != nil {
				return err
			}
			hls = append(hls, uint8(hli))
		}
		cf.HashLens.SeqMatches = hls[0]
		cf.HashLens.Weak = hls[1]
		cf.HashLens.Strong = hls[2]
	case "url":
		cf.URL = val
	case "sha-1":
		byteSlice, err := hex.DecodeString(val)
		if err != nil {
			return err
		}
		if len(byteSlice) != 20 {
			return fmt.Errorf("invalid SHA1 length: got %d, want 20", len(byteSlice))
		}

		copy(cf.SHA1[:], byteSlice)
	default:
		fmt.Println("unkown control file header key")
	}

	return nil
}

func (cf *ControlFile) parseChecksums(reader *bufio.Reader) error {
	weakBytes := cf.HashLens.Weak
	if weakBytes < 1 || weakBytes > 4 {
		return fmt.Errorf("invalid weak checksum length: %d", weakBytes)
	}
	strongBytes := cf.HashLens.Strong
	if strongBytes < 1 || strongBytes > 16 {
		return fmt.Errorf("invalid strong checksum length: %d", strongBytes)
	}
	recordSize := int(weakBytes) + int(strongBytes)
	if recordSize <= 0 {
		return fmt.Errorf("invalid record size")
	}
	
	if len(payload)%recordSize != 0 {
			return fmt.Errorf(
				"payload length %d is not a multiple of record size %d",
				len(payload), recordSize,
			)
		}


	return nil
}
