package system_test

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/cloudfoundry/bosh-utils/logger/loggerfakes"
	. "github.com/cloudfoundry/bosh-utils/system"
	fakesys "github.com/cloudfoundry/bosh-utils/system/fakes"
)

const ErrExitCode = 14

var windowsCommands = map[string]Command{
	"pwd": Command{
		Name:       "powershell",
		Args:       []string{"echo $PWD"},
		WorkingDir: `C:\windows\temp`,
	},
	"stderr": Command{
		Name: "powershell",
		Args: []string{"[Console]::Error.WriteLine('error-output')"},
	},
	"exit": Command{
		Name: "powershell",
		Args: []string{fmt.Sprintf("exit %d", ErrExitCode)},
	},
	"ls": Command{
		Name:       "powershell",
		Args:       []string{"dir"},
		WorkingDir: ".",
	},
	"env": Command{
		Name: "cmd.exe",
		Args: []string{"/C", "SET"},
		Env: map[string]string{
			"FOO": "BAR",
		},
	},
	"echo": Command{
		Name: "powershell",
		Args: []string{"Write-Host", "Hello World!"},
	},
}

var unixCommands = map[string]Command{
	"pwd": Command{
		Name:       "bash",
		Args:       []string{"-c", "echo $PWD"},
		WorkingDir: `/tmp`,
	},
	"stderr": Command{
		Name: "bash",
		Args: []string{"-c", "echo error-output >&2"},
	},
	"exit": Command{
		Name: "bash",
		Args: []string{"-c", fmt.Sprintf("exit %d", ErrExitCode)},
	},
	"ls": Command{
		Name:       "ls",
		Args:       []string{"-l"},
		WorkingDir: ".",
	},
	"env": Command{
		Name: "env",
		Env: map[string]string{
			"FOO": "BAR",
		},
	},
	"echo": Command{
		Name: "echo",
		Args: []string{"Hello World!"},
	},
}

func GetPlatformCommand(cmdName string) Command {
	if runtime.GOOS == "windows" {
		return windowsCommands[cmdName]
	}
	return unixCommands[cmdName]
}

func parseEnvFields(envDump string, convertKeysToUpper bool) map[string]string {
	fields := make(map[string]string)
	envDump = strings.Replace(envDump, "\r", "", -1)
	for _, line := range strings.Split(envDump, "\n") {
		// don't split on '=' as '=' is allowed in the value on Windows
		if n := strings.IndexByte(line, '='); n != -1 {
			key := line[:n]   // key
			val := line[n+1:] // key
			if convertKeysToUpper {
				fields[strings.ToUpper(key)] = val
			} else {
				fields[key] = val
			}
		}
	}
	return fields
}

var _ = Describe("execCmdRunner", func() {
	var (
		runner CmdRunner
	)

	BeforeEach(func() {
		runner = NewExecCmdRunner(boshlog.NewLogger(boshlog.LevelNone))
	})

	Describe("RunComplexCommand", func() {
		It("run complex command with working directory", func() {
			cmd := GetPlatformCommand("ls")
			stdout, stderr, status, err := runner.RunComplexCommand(cmd)
			Expect(err).ToNot(HaveOccurred())
			Expect(stdout).To(ContainSubstring("exec_cmd_runner_fixtures"))
			Expect(stderr).To(BeEmpty())
			Expect(status).To(Equal(0))
		})

		It("run complex command with env", func() {
			cmd := GetPlatformCommand("env")
			cmd.UseIsolatedEnv = false
			stdout, stderr, status, err := runner.RunComplexCommand(cmd)
			Expect(err).ToNot(HaveOccurred())

			envVars := parseEnvFields(stdout, true)
			Expect(envVars).To(HaveKeyWithValue("FOO", "BAR"))
			Expect(envVars).To(HaveKey("PATH"))
			Expect(stderr).To(BeEmpty())
			Expect(status).To(Equal(0))
		})

		It("runs complex command with specific env", func() {
			cmd := GetPlatformCommand("env")
			cmd.UseIsolatedEnv = true
			if runtime.GOOS == "windows" {
				Expect(func() { runner.RunComplexCommand(cmd) }).To(Panic())
			} else {
				stdout, stderr, status, err := runner.RunComplexCommand(cmd)
				Expect(err).ToNot(HaveOccurred())

				envVars := parseEnvFields(stdout, true)
				Expect(envVars).To(HaveKeyWithValue("FOO", "BAR"))
				Expect(envVars).ToNot(HaveKey("PATH"))
				Expect(stderr).To(BeEmpty())
				Expect(status).To(Equal(0))
			}
		})

		setupWindowsEnvTest := func(cmdVars map[string]string) (map[string]string, error) {
			os.Setenv("_FOO", "BAR")
			defer os.Unsetenv("_FOO")

			cmd := GetPlatformCommand("env")
			cmd.Env = cmdVars
			stdout, _, _, err := runner.RunComplexCommand(cmd)
			if err != nil {
				return nil, err
			}

			// don't upper case key names we want to assert that the lower case
			// duplicates provided in Command.Env are used.  also, Windows does
			// not care about key case.
			envVars := parseEnvFields(stdout, false)
			return envVars, nil
		}

		It("uses the env vars specified in the Command", func() {
			envVars, err := setupWindowsEnvTest(map[string]string{
				"_FOO": "BAZZZ",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(envVars).To(HaveKeyWithValue("_FOO", "BAZZZ"))
		})

		It("performs a case-sensitive comparison of env vars when on *Nix", func() {
			if Windows {
				Skip("*Nix only test")
			}
			envVars, err := setupWindowsEnvTest(map[string]string{
				"_foo": "BAZZZ",
				"ABC":  "XYZ",
				"abc":  "xyz",
			})
			Expect(envVars).To(HaveKeyWithValue("_FOO", "BAR"))
			Expect(envVars).To(HaveKeyWithValue("_foo", "BAZZZ"))
			Expect(err).ToNot(HaveOccurred())
			Expect(envVars).To(HaveKeyWithValue("ABC", "XYZ"))
			Expect(envVars).To(HaveKeyWithValue("abc", "xyz"))
		})

		It("env var comparison is case-insensitive on Windows", func() {
			if !Windows {
				Skip("Windows only test")
			}
			envVars, err := setupWindowsEnvTest(map[string]string{
				"_foo": "BAZZZ",
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(envVars).ToNot(HaveKey("_FOO"))
			Expect(envVars).To(HaveKeyWithValue("_foo", "BAZZZ"))
		})

		It("deterministically handles duplicate env vars on Windows", func() {
			if !Windows {
				Skip("Windows only test")
			}
			envVars, err := setupWindowsEnvTest(map[string]string{
				"_foo": "BAZZZ",
				"_bar": "alpha=second",
				"_BAR": "alpha=first",
			})
			Expect(err).ToNot(HaveOccurred())

			// vars in Command.Env replace System vars with the same name,
			// compared case-insensitively. Therefore, the lower case '_foo'
			// replaces the upper case '_FOO'.
			//
			Expect(envVars).ToNot(HaveKey("_FOO"))
			Expect(envVars).To(HaveKeyWithValue("_foo", "BAZZZ"))

			// Duplicate env vars in Command.Env are de-duped before being
			// merged with the System env vars.  Since the Command.Env is
			// a map we sort the keys alphabetically before de-duping so
			// that the result is deterministic.
			//
			Expect(envVars).ToNot(HaveKey("_bar"))
			Expect(envVars).To(HaveKeyWithValue("_BAR", "alpha=first"))
		})

		It("run complex command with stdin", func() {
			input := "This is STDIN\nWith another line."
			cmd := Command{
				Name:  CatExePath,
				Stdin: strings.NewReader(input),
			}
			stdout, stderr, status, err := runner.RunComplexCommand(cmd)
			Expect(err).ToNot(HaveOccurred())
			Expect(stdout).To(Equal(input))
			Expect(stderr).To(BeEmpty())
			Expect(status).To(Equal(0))
		})

		It("prints stdout/stderr to provided I/O object", func() {
			fs := fakesys.NewFakeFileSystem()
			stdoutFile, err := fs.OpenFile("/fake-stdout-path", os.O_RDWR, os.FileMode(0644))
			Expect(err).ToNot(HaveOccurred())

			stderrFile, err := fs.OpenFile("/fake-stderr-path", os.O_RDWR, os.FileMode(0644))
			Expect(err).ToNot(HaveOccurred())

			cmd := Command{
				Name:   CatExePath,
				Args:   []string{"-stdout", "fake-out", "-stderr", "fake-err"},
				Stdout: stdoutFile,
				Stderr: stderrFile,
			}

			stdout, stderr, status, err := runner.RunComplexCommand(cmd)
			Expect(err).ToNot(HaveOccurred())

			Expect(stdout).To(BeEmpty())
			Expect(stderr).To(BeEmpty())
			Expect(status).To(Equal(0))

			stdoutContents := make([]byte, 1024)
			_, err = stdoutFile.Read(stdoutContents)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(stdoutContents)).To(ContainSubstring("fake-out"))

			stderrContents := make([]byte, 1024)
			_, err = stderrFile.Read(stderrContents)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(stderrContents)).To(ContainSubstring("fake-err"))
		})
	})

	Describe("RunComplexCommandAsync", func() {
		It("populates stdout and stderr", func() {
			cmd := GetPlatformCommand("ls")
			process, err := runner.RunComplexCommandAsync(cmd)
			Expect(err).ToNot(HaveOccurred())

			result := <-process.Wait()
			Expect(result.Error).ToNot(HaveOccurred())
			Expect(result.ExitStatus).To(Equal(0))
		})

		It("populates stdout and stderr", func() {
			cmd := Command{
				Name: CatExePath,
				Args: []string{"-stdout", "STDOUT", "-stderr", "STDERR"},
			}
			process, err := runner.RunComplexCommandAsync(cmd)
			Expect(err).ToNot(HaveOccurred())

			result := <-process.Wait()
			Expect(result.Error).ToNot(HaveOccurred())
			Expect(result.Stdout).To(Equal("STDOUT\n"))
			Expect(result.Stderr).To(Equal("STDERR\n"))
		})

		It("returns error and sets status to exit status of command if it exits with non-0 status", func() {
			cmd := GetPlatformCommand("exit")
			process, err := runner.RunComplexCommandAsync(cmd)
			Expect(err).ToNot(HaveOccurred())

			result := <-process.Wait()
			Expect(result.Error).To(HaveOccurred())
			Expect(result.ExitStatus).To(Equal(ErrExitCode))
		})

		It("allows setting custom env variable in addition to inheriting process env variables", func() {
			cmd := GetPlatformCommand("env")
			cmd.UseIsolatedEnv = false

			process, err := runner.RunComplexCommandAsync(cmd)
			Expect(err).ToNot(HaveOccurred())

			result := <-process.Wait()
			Expect(result.Error).ToNot(HaveOccurred())
			Expect(result.Stdout).To(ContainSubstring("FOO=BAR"))
			Expect(result.Stdout).To(ContainSubstring("PATH="))
		})

		It("changes working dir", func() {
			cmd := GetPlatformCommand("pwd")
			process, err := runner.RunComplexCommandAsync(cmd)
			Expect(err).ToNot(HaveOccurred())

			result := <-process.Wait()
			Expect(result.Error).ToNot(HaveOccurred())
			Expect(result.Stdout).To(ContainSubstring(cmd.WorkingDir))
		})
	})

	Describe("RunCommand", func() {
		It("run command", func() {
			cmd := GetPlatformCommand("echo")
			stdout, stderr, status, err := runner.RunCommand(cmd.Name, cmd.Args...)
			Expect(err).ToNot(HaveOccurred())
			Expect(stdout).To(Equal("Hello World!\n"))
			Expect(stderr).To(BeEmpty())
			Expect(status).To(Equal(0))
		})

		It("run command with error output", func() {
			cmd := GetPlatformCommand("stderr")
			stdout, stderr, status, err := runner.RunCommand(cmd.Name, cmd.Args...)
			Expect(err).ToNot(HaveOccurred())
			Expect(stdout).To(BeEmpty())
			Expect(stderr).To(ContainSubstring("error-output"))
			Expect(status).To(Equal(0))
		})

		It("run command with non-0 exit status", func() {
			cmd := GetPlatformCommand("exit")
			stdout, stderr, status, err := runner.RunCommand(cmd.Name, cmd.Args...)
			Expect(err).To(HaveOccurred())
			Expect(stdout).To(BeEmpty())
			Expect(stderr).To(BeEmpty())
			Expect(status).To(Equal(ErrExitCode))
		})

		It("run command with error", func() {
			stdout, stderr, status, err := runner.RunCommand(FalseExePath)
			Expect(err).To(HaveOccurred())
			Expect(stderr).To(BeEmpty())
			Expect(stdout).To(BeEmpty())
			Expect(status).To(Equal(1))
		})

		It("run command with error with args", func() {
			stdout, stderr, status, err := runner.RunCommand(FalseExePath, "second arg")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(fmt.Sprintf("Running command: '%s second arg', stdout: '', stderr: '': exit status 1", FalseExePath)))
			Expect(stderr).To(BeEmpty())
			Expect(stdout).To(BeEmpty())
			Expect(status).To(Equal(1))
		})

		It("run command with cmd not found", func() {
			stdout, stderr, status, err := runner.RunCommand("something that does not exist")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Or(ContainSubstring("not found"), ContainSubstring("ObjectNotFound")))
			if runtime.GOOS != "windows" {
				Expect(stderr).To(BeEmpty())
			}
			Expect(stdout).To(BeEmpty())
			Expect(status).ToNot(Equal(0))
		})
	})

	Describe("RunCommandWithInput", func() {
		It("run command with input", func() {
			stdout, stderr, status, err := runner.RunCommandWithInput("foo\nbar\nbaz", CatExePath)
			Expect(err).ToNot(HaveOccurred())
			Expect(stdout).To(Equal("foo\nbar\nbaz"))
			Expect(stderr).To(BeEmpty())
			Expect(status).To(Equal(0))
		})
	})

	Describe("RunCommandQuietly", func() {
		It("run command with input", func() {
			logger := &loggerfakes.FakeLogger{}
			runner = NewExecCmdRunner(logger)

			cmd := GetPlatformCommand("echo")
			stdout, stderr, status, err := runner.RunCommandQuietly(cmd.Name, cmd.Args...)
			Expect(err).ToNot(HaveOccurred())
			Expect(logger.DebugCallCount()).To(Equal(2))
			Expect(stdout).To(Equal("Hello World!\n"))
			Expect(stderr).To(BeEmpty())
			Expect(status).To(Equal(0))
		})
	})

	Describe("CommandExists", func() {
		It("command exists", func() {
			var cmd string
			if runtime.GOOS == "windows" {
				cmd = "cmd.exe"
			} else {
				cmd = "env"
			}
			Expect(runner.CommandExists(cmd)).To(BeTrue())
			Expect(runner.CommandExists("absolutely-does-not-exist-ever-please-unicorns")).To(BeFalse())
		})
	})
})
