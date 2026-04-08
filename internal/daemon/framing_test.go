package daemon

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteReadJSON_RoundTrip(t *testing.T) {
	type msg struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	original := msg{Name: "hello", Value: 42}

	var buf bytes.Buffer
	err := WriteJSON(&buf, original)
	require.NoError(t, err)

	var decoded msg
	err = ReadJSON(&buf, &decoded)
	require.NoError(t, err)
	require.Equal(t, original, decoded)
}

func TestWriteReadJSON_MultipleMessages(t *testing.T) {
	var buf bytes.Buffer

	msgs := []Request{
		{ID: 1, Method: "status"},
		{ID: 2, Method: "peers"},
		{ID: 3, Method: "catalog"},
	}

	for _, m := range msgs {
		require.NoError(t, WriteJSON(&buf, m))
	}

	for _, expected := range msgs {
		var got Request
		require.NoError(t, ReadJSON(&buf, &got))
		require.Equal(t, expected.ID, got.ID)
		require.Equal(t, expected.Method, got.Method)
	}
}

func TestReadJSON_EmptyReader(t *testing.T) {
	var buf bytes.Buffer
	var msg Request
	err := ReadJSON(&buf, &msg)
	require.Error(t, err)
}
