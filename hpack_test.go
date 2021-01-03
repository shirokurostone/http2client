package main

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeInteger(t *testing.T) {

	testcases := []struct {
		value    int
		n        int
		expected []byte
	}{
		{10, 5, []byte{0x0a}},
		{1337, 5, []byte{0x1f, 0x9a, 0x0a}},
		{42, 8, []byte{0x2a}},
	}

	for i := 0; i < len(testcases); i++ {
		c := testcases[i]
		actual := EncodeInteger(c.value, c.n)
		assert.Equal(t, c.expected, actual)
	}
}

func TestDecodeInteger(t *testing.T) {

	testcases := []struct {
		input    []byte
		n        int
		expected int
	}{
		{[]byte{0x0a}, 5, 10},
		{[]byte{0x1f, 0x9a, 0x0a}, 5, 1337},
		{[]byte{0x2a}, 8, 42},
	}

	for i := 0; i < len(testcases); i++ {
		c := testcases[i]
		actual := DecodeInteger(c.input, c.n)
		assert.Equal(t, c.expected, actual)
	}
}

func TestEncodeHuffmanCode(t *testing.T) {
	testcases := []struct {
		input  string
		output string
	}{
		{"www.example.com", "f1e3c2e5f23a6ba0ab90f4ff"},
		{"no-cache", "a8eb10649cbf"},
		{"custom-key", "25a849e95ba97d7f"},
		{"custom-value", "25a849e95bb8e8b4bf"},
		{"private", "aec3771a4b"},
		{"Mon, 21 Oct 2013 20:13:21 GMT", "d07abe941054d444a8200595040b8166e082a62d1bff"},
		{"https://www.example.com", "9d29ad171863c78f0b97c8e9ae82ae43d3"},
	}

	for i := 0; i < len(testcases); i++ {
		c := testcases[i]
		expected, err := hex.DecodeString(c.output)
		assert.Nil(t, err)
		actual := EncodeHuffmanCode(c.input, false)
		assert.Equal(t, expected, actual)
	}
}

func TestDecodeHuffmanCode(t *testing.T) {
	testcases := []struct {
		expected string
		input    string
	}{
		{"www.example.com", "f1e3c2e5f23a6ba0ab90f4ff"},
		{"no-cache", "a8eb10649cbf"},
		{"custom-key", "25a849e95ba97d7f"},
		{"custom-value", "25a849e95bb8e8b4bf"},
		{"private", "aec3771a4b"},
		{"Mon, 21 Oct 2013 20:13:21 GMT", "d07abe941054d444a8200595040b8166e082a62d1bff"},
		{"https://www.example.com", "9d29ad171863c78f0b97c8e9ae82ae43d3"},
	}

	for i := 0; i < len(testcases); i++ {
		c := testcases[i]
		input, err := hex.DecodeString(c.input)
		assert.Nil(t, err)
		actual := DecodeHuffmanCode(input)
		assert.Equal(t, c.expected, actual)
	}
}
