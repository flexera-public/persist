// Copyright (c) 2015 RightScale, Inc. - see LICENSE

package persist

// Omega: Alt+937

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/inconshreveable/log15.v2"
)

// log client used for testing
type testLogClient struct {
	i    int  // log generation, incremented to distinguing rotations
	n    int  // count number of objects replayed
	intr bool // interrupt persistAll
}

// verifies that what's being replayed is what we expect
func (tlc *testLogClient) Replay(ev interface{}) error {
	log15.Info("Replay called", "ev", ev, "i", tlc.i, "n", tlc.n)
	Ω(tlc.n).Should(BeNumerically("<", 3))
	switch tlc.n {
	case 0:
		Ω(ev).Should(Equal(&logEv1{S: fmt.Sprintf("hello world #%d!", tlc.i)}))
	case 1:
		Ω(ev).Should(Equal(&logEv2{A: 55 + tlc.i, B: "Hello Again"}))
	case 2:
		Ω(ev).Should(Equal(&logEv1{S: "not again!"}))
	}
	tlc.n++
	return nil
}

// persist some new data to the new log...
func (tlc *testLogClient) PersistAll(pl Log) {
	log15.Info("Populating log", "i", tlc.i)
	if tlc.intr {
		pl.(*pLog).Close()
		return
	}
	Ω(pl.Output(&logEv1{S: fmt.Sprintf("hello world #%d!", tlc.i+1)})).ShouldNot(HaveOccurred())
	Ω(pl.Output(&logEv2{A: 55 + tlc.i + 1, B: "Hello Again"})).ShouldNot(HaveOccurred())
	Ω(pl.Output(&logEv1{S: "not again!"})).ShouldNot(HaveOccurred())
}

// custom even types written to the log
type logEv1 struct {
	S string
}
type logEv2 struct {
	A int
	B string
}

func init() {
	Register(&logEv1{})
	Register(&logEv2{})
}

var _ = Describe("NewLog", func() {

	BeforeEach(func() {
		os.RemoveAll(PT)
		os.Mkdir(PT, 0777)
	})
	AfterEach(func() {
		//os.RemoveAll(PT)
	})

	startNewLog := func(i int, intr bool) (Log, LogClient) {
		fd, err := NewFileDest(PT+"/newfile", true, nil)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(fd).ShouldNot(BeNil())

		logClient := testLogClient{i: i, intr: intr}
		pl, err := NewLog(fd, &logClient, log15.Root())
		// if we interrupt the replay we're breaking the log
		// (intentionally) and so we do expect to get an error
		if !intr {
			Ω(err).ShouldNot(HaveOccurred())
			Ω(pl).ShouldNot(BeNil())
		} else {
			Ω(err).Should(HaveOccurred())
			Ω(pl).Should(BeNil())
		}

		return pl, &logClient
	}

	rereadLog := func(oldI int) Log {
		pl, lc := startNewLog(oldI, false)
		Ω(lc.(*testLogClient).n).Should(Equal(3))
		return pl
	}

	rereadLogInterrupted := func(oldI int) {
		pl, lc := startNewLog(oldI, true)
		Ω(lc.(*testLogClient).n).Should(Equal(3))
		Ω(pl).Should(BeNil())
	}

	It("verifies a new log file", func() {
		By("starting a new log")
		pl, _ := startNewLog(0, false)
		pl.(*pLog).Close()

		By("re-reading the log")
		pl = rereadLog(1)
		pl.(*pLog).Close()
	})

	It("verifies a new incomplete log file", func() {
		By("starting a new log")
		pl, _ := startNewLog(0, false)
		pl.(*pLog).Close()

		By("re-reading the log")
		rereadLogInterrupted(1)

		By("re-reading the log again")
		pl = rereadLog(1)
		pl.(*pLog).Close()

		By("re-reading the log yet again")
		pl = rereadLog(2)
		pl.(*pLog).Close()
	})

	/*
		It("verifies log rotation", func() {
			pl, _ := startNewLog(0, false)
			// set a small size limit so we get it to rotate
			pl.SetSizeLimit(100)
			for i := 0; i <= 100; i++ {
				data := logEv2{B: "A log event", A: i}
				pl.Output(&data)
			}
			//

		})
	*/

})
