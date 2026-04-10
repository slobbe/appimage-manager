package zsync

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestParseControlFileParsesHeaderAndChecksums(t *testing.T) {
	sha1Hex := "00112233445566778899aabbccddeeff00112233"
	rawSHA1, err := hex.DecodeString(sha1Hex)
	if err != nil {
		t.Fatalf("DecodeString returned error: %v", err)
	}

	control := "" +
		"Filename: sample.AppImage\n" +
		"Blocksize: 2048\n" +
		"Length: 4096\n" +
		"Hash-Lengths: 2,2,3\n" +
		"URL: https://example.com/sample.AppImage\n" +
		"SHA-1: " + sha1Hex + "\n" +
		"\n"
	payload := []byte{
		0x01, 0x02, 0xaa, 0xbb, 0xcc,
		0x03, 0x04, 0xdd, 0xee, 0xff,
	}

	cf, err := ParseControlFile(bytes.NewReader(append([]byte(control), payload...)))
	if err != nil {
		t.Fatalf("ParseControlFile returned error: %v", err)
	}

	if cf.Filename != "sample.AppImage" {
		t.Fatalf("Filename = %q, want %q", cf.Filename, "sample.AppImage")
	}
	if cf.BlockSize != 2048 {
		t.Fatalf("BlockSize = %d, want %d", cf.BlockSize, 2048)
	}
	if cf.Length != 4096 {
		t.Fatalf("Length = %d, want %d", cf.Length, 4096)
	}
	if cf.URL != "https://example.com/sample.AppImage" {
		t.Fatalf("URL = %q", cf.URL)
	}
	if cf.HashLens.SeqMatches != 2 || cf.HashLens.Weak != 2 || cf.HashLens.Strong != 3 {
		t.Fatalf("HashLens = %+v", cf.HashLens)
	}
	if cf.SHA1 != [20]byte(rawSHA1) {
		t.Fatalf("SHA1 = %x, want %x", cf.SHA1, rawSHA1)
	}
	if len(cf.Blocks) != 2 {
		t.Fatalf("len(Blocks) = %d, want %d", len(cf.Blocks), 2)
	}
	if cf.Blocks[0].Weak != 0x0102 {
		t.Fatalf("Blocks[0].Weak = %#x, want %#x", cf.Blocks[0].Weak, 0x0102)
	}
	if !bytes.Equal(cf.Blocks[0].Strong, []byte{0xaa, 0xbb, 0xcc}) {
		t.Fatalf("Blocks[0].Strong = %x", cf.Blocks[0].Strong)
	}
	if cf.Blocks[1].Weak != 0x0304 {
		t.Fatalf("Blocks[1].Weak = %#x, want %#x", cf.Blocks[1].Weak, 0x0304)
	}
	if !bytes.Equal(cf.Blocks[1].Strong, []byte{0xdd, 0xee, 0xff}) {
		t.Fatalf("Blocks[1].Strong = %x", cf.Blocks[1].Strong)
	}
}

func TestParseControlFileRejectsMismatchedPayloadLength(t *testing.T) {
	control := "" +
		"Filename: sample.AppImage\n" +
		"Blocksize: 2048\n" +
		"Length: 4096\n" +
		"Hash-Lengths: 2,2,3\n" +
		"URL: https://example.com/sample.AppImage\n" +
		"SHA-1: 00112233445566778899aabbccddeeff00112233\n" +
		"\n"
	payload := []byte{0x01, 0x02, 0xaa}

	_, err := ParseControlFile(bytes.NewReader(append([]byte(control), payload...)))
	if err == nil {
		t.Fatal("ParseControlFile returned nil error, want payload length error")
	}
}
