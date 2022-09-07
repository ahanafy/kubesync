package gcpapi

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdempotentCreateRemoteSecret_Error(t *testing.T) {
	projectID := ""
	secretName := ""
	payload := make([]byte, 0)
	g := new(GCPCreds)
	err := g.idempotentCreateRemoteSecret(projectID, secretName, payload)
	if err != nil {
		fmt.Println(err)
	}

	// Error shouldn't be nil
	assert.NotNil(t, err)
}
