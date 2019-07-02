package healthchecks

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestHealthchecks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Healthchecks Suite")
}
