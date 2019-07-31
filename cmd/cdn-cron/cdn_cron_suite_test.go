package main_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCdnCron(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CdnCron Suite")
}
