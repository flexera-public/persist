// Copyright (c) 2015 RightScale, Inc. - see LICENSE

package persist

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"gopkg.in/inconshreveable/log15.v2"
)

func TestUCA(t *testing.T) {
	//log.SetOutput(ginkgo.GinkgoWriter)
	log15.Root().SetHandler(log15.StreamHandler(GinkgoWriter, log15.TerminalFormat()))

	format.UseStringerRepresentation = true
	RegisterFailHandler(Fail)

	RunSpecs(t, "persist")
}
