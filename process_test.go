package process_test

import (
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	process "github.com/Originate/go-process"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// ByFullOutput is used to sort output chunks for the case when
// the send of two chunks to the channel block and
// thus may be received in any order
type ByFullOutput []process.OutputChunk

func (b ByFullOutput) Len() int {
	return len(b)
}
func (b ByFullOutput) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
func (b ByFullOutput) Less(i, j int) bool {
	return len(b[i].Full) < len(b[j].Full)
}

var _ = Describe("Process", func() {
	It("returns no errors when the process succeeds", func() {
		p := process.NewProcess("./test_executables/passing")
		err := p.Run()
		Expect(err).To(BeNil())
	})

	It("returns errors when the process fails", func() {
		p := process.NewProcess("./test_executables/failing")
		err := p.Run()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(Equal("exit status 1"))
	})

	It("captures output", func() {
		p := process.NewProcess("./test_executables/passing")
		err := p.Run()
		Expect(err).To(BeNil())
		Expect(p.Output).To(Equal("output"))
	})

	It("allows settings of the current working directory", func() {
		cwd, err := os.Getwd()
		customDir := path.Join(cwd, "test_executables")
		Expect(err).To(BeNil())
		p := process.NewProcess("./print_cwd")
		p.SetDir(customDir)
		err = p.Run()
		Expect(err).To(BeNil())
		Expect(p.Output).To(Equal(customDir))
	})

	It("allows settings of the env variables", func() {
		p := process.NewProcess("./test_executables/print_env")
		p.SetEnv([]string{"MY_VAR=special"})
		err := p.Run()
		Expect(err).To(BeNil())
		Expect(p.Output).To(Equal("special"))
	})

	It("allows killing long running processes", func() {
		p := process.NewProcess("./test_executables/output_chunks")
		err := p.Start()
		Expect(err).To(BeNil())
		time.Sleep(time.Second)
		err = p.Kill()
		Expect(err).To(BeNil())
		Expect(p.Output).NotTo(ContainSubstring("late chunk 4"))
	})

	It("allows waiting for long running processes", func() {
		p := process.NewProcess("./test_executables/output_chunks")
		err := p.Start()
		Expect(err).To(BeNil())
		err = p.Wait()
		Expect(err).To(BeNil())
		Expect(p.Output).To(ContainSubstring("late chunk 4"))
	})

	Describe("output channel", func() {
		It("allows access to output chunks (separated by newlines) via a channel", func() {
			p := process.NewProcess("./test_executables/output_chunks")
			outputChannel, _ := p.GetOutputChannel()
			err := p.Start()
			Expect(err).To(BeNil())
			chunks := []process.OutputChunk{
				<-outputChannel,
				<-outputChannel,
				<-outputChannel,
				<-outputChannel,
				<-outputChannel,
			}
			sort.Sort(ByFullOutput(chunks))
			Expect(chunks).To(Equal([]process.OutputChunk{
				{Chunk: "", Full: ""},
				{Chunk: "chunk 1", Full: "chunk 1"},
				{Chunk: "special chunk 2", Full: "chunk 1\nspecial chunk 2"},
				{Chunk: "chunk 3", Full: "chunk 1\nspecial chunk 2\nchunk 3"},
				{Chunk: "late chunk 4", Full: "chunk 1\nspecial chunk 2\nchunk 3\nlate chunk 4"},
			}))
			err = p.Kill()
			Expect(err).To(BeNil())
		})

		It("sends the current status whenever the channel is added", func() {
			p := process.NewProcess("./test_executables/output_chunks")
			err := p.Start()
			Expect(err).To(BeNil())
			time.Sleep(time.Second)
			outputChannel, _ := p.GetOutputChannel()
			chunk := <-outputChannel
			Expect(chunk).To(Equal(process.OutputChunk{Chunk: "", Full: "chunk 1\nspecial chunk 2\nchunk 3"}))
			err = p.Kill()
			Expect(err).To(BeNil())
		})
	})

	Describe("waitForCondition", func() {
		It("returns nil if the condition passes within the timeout", func() {
			p := process.NewProcess("./test_executables/output_chunks")
			err := p.Start()
			Expect(err).To(BeNil())
			err = p.WaitForCondition(func(chunk, full string) bool {
				return strings.Contains(chunk, "special")
			}, time.Second)
			Expect(err).To(BeNil())
			err = p.Kill()
			Expect(err).To(BeNil())
		})

		It("returns error if the text is not seen within the timeout", func() {
			p := process.NewProcess("./test_executables/output_chunks")
			err := p.Start()
			Expect(err).To(BeNil())
			err = p.WaitForCondition(func(chunk, full string) bool {
				return strings.Contains(chunk, "other")
			}, time.Second)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Timed out after 1s, full output:\nchunk 1\nspecial chunk 2\nchunk 3"))
			err = p.Kill()
			Expect(err).To(BeNil())
		})
	})

	Describe("waitForRegexp", func() {
		It("returns nil if the text is seen within the timeout", func() {
			p := process.NewProcess("./test_executables/output_chunks")
			err := p.Start()
			Expect(err).To(BeNil())
			isChunk := regexp.MustCompile(`special chunk \d`)
			err = p.WaitForRegexp(isChunk, time.Second)
			Expect(err).To(BeNil())
			err = p.Kill()
			Expect(err).To(BeNil())
		})

		It("returns error if the text is not seen within the timeout", func() {
			p := process.NewProcess("./test_executables/output_chunks")
			err := p.Start()
			Expect(err).To(BeNil())
			isChunk := regexp.MustCompile(`other chunk \d`)
			err = p.WaitForRegexp(isChunk, time.Second)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Timed out after 1s, full output:\nchunk 1\nspecial chunk 2\nchunk 3"))
			err = p.Kill()
			Expect(err).To(BeNil())
		})
	})

	Describe("waitForText", func() {
		It("returns nil if the text is seen within the timeout", func() {
			p := process.NewProcess("./test_executables/output_chunks")
			err := p.Start()
			Expect(err).To(BeNil())
			err = p.WaitForText("chunk 3", time.Second)
			Expect(err).To(BeNil())
			err = p.Kill()
			Expect(err).To(BeNil())
		})

		It("returns error if the text is not seen within the timeout", func() {
			p := process.NewProcess("./test_executables/output_chunks")
			err := p.Start()
			Expect(err).To(BeNil())
			err = p.WaitForText("chunk 4", time.Second)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("Timed out after 1s, full output:\nchunk 1\nspecial chunk 2\nchunk 3"))
			err = p.Kill()
			Expect(err).To(BeNil())
		})

		It("works for prompts (text that ends with a colon followed by a space)", func() {
			p := process.NewProcess("./test_executables/prompt")
			err := p.Start()
			Expect(err).To(BeNil())
			err = p.WaitForText("prompt: ", time.Second)
			Expect(err).To(BeNil())
			err = p.Kill()
			Expect(err).To(BeNil())
		})
	})
})
