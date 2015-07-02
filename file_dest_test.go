// Copyright (c) 2015 RightScale, Inc. - see LICENSE

package persist

// Omega: Alt+937

import (
	"io"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/inconshreveable/log15.v2"
)

const PT = "/tmp/persist_test"

var _ = Describe("NewFileDest", func() {

	BeforeEach(func() {
		os.RemoveAll(PT)
		os.Mkdir(PT, 0777)
	})
	AfterEach(func() { os.RemoveAll(PT) })

	It("rejects invalid chars", func() {
		f, err := NewFileDest(PT+"/test.plog", true, nil)
		Ω(f).Should(BeNil())
		Ω(err).Should(HaveOccurred())
		Ω(err.Error()).Should(ContainSubstring("cannot contain"))
	})

	It("handles an invalid directory", func() {
		f, err := NewFileDest(PT+"/xxx/test", true, nil)
		Ω(f).Should(BeNil())
		Ω(err).Should(HaveOccurred())
		Ω(err.Error()).Should(ContainSubstring("Cannot create"))
	})

	It("does not create log without create=true", func() {
		f, err := NewFileDest(PT+"/test", false, nil)
		Ω(f).Should(BeNil())
		Ω(err).Should(HaveOccurred())
		Ω(err.Error()).Should(ContainSubstring("No existing"))
	})
})

var _ = Describe("FileDest", func() {

	BeforeEach(func() {
		os.RemoveAll(PT)
		os.Mkdir(PT, 0777)
	})
	AfterEach(func() {
		//os.RemoveAll(PT)
	})

	startNewLog := func() LogDestination {
		By("starting a new log")

		fd, err := NewFileDest(PT+"/newfile", true, nil)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(fd).ShouldNot(BeNil())

		buf := make([]byte, 100)
		n, err := fd.Read(buf)
		Ω(err).Should(Equal(io.EOF))
		Ω(n).Should(Equal(0))

		Ω(fd.EndRotate()).ShouldNot(HaveOccurred())

		n, err = fd.Write([]byte("Hello World"))
		Ω(err).ShouldNot(HaveOccurred())
		Ω(n).Should(Equal(11))

		n, err = fd.Write([]byte("Hello Again"))
		Ω(err).ShouldNot(HaveOccurred())
		Ω(n).Should(Equal(11))

		return fd
	}

	rereadLog := func() LogDestination {
		By("re-reading the log file")

		fd, err := NewFileDest(PT+"/newfile", false, nil)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(fd).ShouldNot(BeNil())

		buf := make([]byte, 100)
		n, err := fd.Read(buf)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(n).Should(Equal(22))
		Ω(buf[:n]).Should(Equal([]byte("Hello WorldHello Again")))

		n, err = fd.Read(buf)
		if err == nil {
			log15.Warn("Read not EOF", "n", n)
		}
		Ω(err).Should(Equal(io.EOF))
		Ω(n).Should(Equal(0))

		return fd
	}

	rereadLog2x := func() LogDestination {
		By("re-reading 2x the log file")

		fd, err := NewFileDest(PT+"/newfile", false, nil)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(fd).ShouldNot(BeNil())

		buf := make([]byte, 100)
		// replays the first log file
		n, err := fd.Read(buf)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(n).Should(Equal(22))
		Ω(buf[:n]).Should(Equal([]byte("Hello WorldHello Again")))

		// replays the second log file
		n, err = fd.Read(buf)
		Ω(err).ShouldNot(HaveOccurred())
		Ω(n).Should(Equal(22))
		Ω(buf[:n]).Should(Equal([]byte("Hello WorldHello Again")))

		n, err = fd.Read(buf)
		Ω(err).Should(Equal(io.EOF))
		Ω(n).Should(Equal(0))

		return fd
	}

	rotateLog := func(fd LogDestination) {
		By("rotating the log file")

		err := fd.StartRotate()
		Ω(err).ShouldNot(HaveOccurred())

		n, err := fd.Write([]byte("Hello World"))
		Ω(err).ShouldNot(HaveOccurred())
		Ω(n).Should(Equal(11))

		n, err = fd.Write([]byte("Hello Again"))
		Ω(err).ShouldNot(HaveOccurred())
		Ω(n).Should(Equal(11))

	}

	It("verifies a new log file", func() {
		fd := startNewLog()
		fd.Close()

		fd = rereadLog()
		Ω(fd.EndRotate()).ShouldNot(HaveOccurred())
		fd.Close()
	})

	It("verifies a new log file twice", func() {
		fd := startNewLog()
		fd.Close()

		fd = rereadLog()

		n, err := fd.Write([]byte("Hello World"))
		Ω(err).ShouldNot(HaveOccurred())
		Ω(n).Should(Equal(11))

		Ω(fd.EndRotate()).ShouldNot(HaveOccurred())

		n, err = fd.Write([]byte("Hello Again"))
		Ω(err).ShouldNot(HaveOccurred())
		Ω(n).Should(Equal(11))

		fd.Close()

		fd = rereadLog()
		Ω(fd.EndRotate()).ShouldNot(HaveOccurred())
		fd.Close()
	})

	It("verifies a new incomplete log file", func() {
		fd := startNewLog()
		fd.Close()

		fd = rereadLog()

		n, err := fd.Write([]byte("Hello World"))
		Ω(err).ShouldNot(HaveOccurred())
		Ω(n).Should(Equal(11))

		n, err = fd.Write([]byte("Hello Again"))
		Ω(err).ShouldNot(HaveOccurred())
		Ω(n).Should(Equal(11))

		fd.Close()

		fd = rereadLog2x()

		Ω(fd.EndRotate()).ShouldNot(HaveOccurred())
		fd.Close()
	})

	It("verifies log rotation", func() {
		fd := startNewLog()
		rotateLog(fd)
		fd = rereadLog2x()
		Ω(fd.EndRotate()).ShouldNot(HaveOccurred())
		fd.Close()
	})

})
