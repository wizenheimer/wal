package wal

import "google.golang.org/protobuf/proto"

// MustMarshal marshals a WAL_Entry to a byte slice
// It panics if the marshalling fails
func MustMarshal(entry *WAL_Entry) []byte {
	marshalled, err := proto.Marshal(entry)
	if err != nil {
		panic(err)
	}
	return marshalled
}

// MustUnmarshal unmarshals a byte slice to a WAL_Entry
// It panics if the unmarshalling fails
func MustUnmarshal(data []byte, entry *WAL_Entry) {
	err := proto.Unmarshal(data, entry)
	if err != nil {
		panic(err)
	}
}
