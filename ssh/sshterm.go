package ssh

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/blacknon/gexpect"
	"github.com/blacknon/lssh/conf"
)

type ConInfoTerm struct {
	Log         bool
	LogDir      string
	Server      string
	Addr        string
	Port        string
	User        string
	Pass        string
	KeyPath     string
	BeforeCmd   string
	AfterCmd    string
	ProxyServer string
	Proxy       conf.ProxyConfig
	LocalUser   *user.User
}

func (c *ConInfoTerm) runBeforeCmd() {
	if c.BeforeCmd != "" {
		out, _ := exec.Command("sh", "-c", c.BeforeCmd).CombinedOutput()
		fmt.Println(string(out))
	}
}

func (c *ConInfoTerm) runAfterCmd() {
	if c.AfterCmd != "" {
		out, _ := exec.Command("sh", "-c", c.AfterCmd).CombinedOutput()
		fmt.Println(string(out))
	}
}

func (c *ConInfoTerm) createSshCmd() (sshCmd []string) {
	// Default(Password Auth)
	// ssh command Args
	// "/usr/bin/ssh -o 'StrictHostKeyChecking no' -o 'NumberOfPasswordPrompts 1' connectUser@connectAddr -p connectPort"
	sshCmd = []string{"/usr/bin/ssh",
		"-o", "StrictHostKeyChecking no",
		"-o", "NumberOfPasswordPrompts 1",
		c.User + "@" + c.Addr,
		"-p", c.Port}

	// Key Auth
	if c.KeyPath != "" {
		c.KeyPath = strings.Replace(c.KeyPath, "~", c.LocalUser.HomeDir, 1)
		// ssh command Args
		// "/usr/bin/ssh -o 'StrictHostKeyChecking no' -o 'NumberOfPasswordPrompts 1' -i connectKey connectUser@connectAddr -p connectPort"
		sshCmd = []string{"/usr/bin/ssh",
			"-o", "StrictHostKeyChecking no",
			"-o", "NumberOfPasswordPrompts 1",
			"-i", c.KeyPath,
			c.User + "@" + c.Addr,
			"-p", c.Port}
	}

	// setup ssh Proxy
	if c.ProxyServer != "" {
		proxyHost := c.Proxy.User + "@" + c.Proxy.Addr

		// Create ProxyCommand(password auth)
		proxyCommand := "ProxyCommand=ssh -W %h:%p " + proxyHost
		if c.Proxy.Key != "" {
			// Create ProxyCommand(key auth)
			proxyCommand = "ProxyCommand=ssh -W %h:%p -i " + c.Proxy.Key + " " + proxyHost
		}

		proxyOption := []string{"-o", proxyCommand}
		sshCmd = append(sshCmd, proxyOption...)
	}
	return
}

func (c *ConInfoTerm) Connect() (err error) {
	fmt.Println(c.ProxyServer)
	// Set default port
	if c.Port == "" {
		c.Port = "22"
	}
	c.LocalUser, _ = user.Current()

	// Run and set Before/After Command
	c.runBeforeCmd()
	defer c.runAfterCmd()

	// Create ssh command
	sshCmd := c.createSshCmd()

	// exec ssh command
	child, _ := gexpect.NewSubProcess(sshCmd[0], sshCmd[1:]...)

	// Log Enable
	if c.Log == true {
		logDirPath := c.LogDir
		logDirPath = strings.Replace(logDirPath, "~", c.LocalUser.HomeDir, 1)
		logDirPath = strings.Replace(logDirPath, "<Date>", time.Now().Format("20060102"), 1)
		logDirPath = strings.Replace(logDirPath, "<Hostname>", c.Server, 1)

		// mkdir logDIr
		if err := os.MkdirAll(logDirPath, 0700); err != nil {
			return err
		}

		// Golang time.format YYYYmmdd_HHMMSS = "20060102_150405".(https://golang.org/src/time/format.go)
		logFile := time.Now().Format("20060102_150405") + "_" + c.Server + ".log"
		logFilePATH := logDirPath + "/" + logFile
		fifoPATH := logDirPath + "/." + logFile + ".fifo"

		// Create FIFO
		syscall.Mknod(fifoPATH, syscall.S_IFIFO|0600, 0)
		defer os.Remove(fifoPATH)

		// log write FIFO
		fifoWrite, _ := os.OpenFile(fifoPATH, os.O_RDWR|os.O_APPEND, 0600)
		child.Term.Log = fifoWrite

		// Read FiIFO write LogFile Add Timestamp
		go func() {
			for {
				// Open FIFO
				openFIFO, err := os.Open(fifoPATH)
				if err != nil {
					return
				}
				scanner := bufio.NewScanner(openFIFO)

				// Open Logfile
				wirteLog, err := os.OpenFile(logFilePATH, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
				if err != nil {
					return
				}

				// for loop Add timestamp log
				for scanner.Scan() {
					fmt.Fprintln(wirteLog, time.Now().Format("2006/01/02 15:04:05 ")+scanner.Text())
				}
				wirteLog.Close()
			}
		}()
	}

	// gexpect start
	if err := child.Start(); err != nil {
		return err
	}
	defer child.Close()

	// Terminal Size Change Trap
	signal_chan := make(chan os.Signal, 1)
	signal.Notify(signal_chan, syscall.SIGWINCH)
	go func() {
		for {
			s := <-signal_chan
			switch s {
			case syscall.SIGWINCH:
				child.Term.ResetWinSize()
			}
		}
	}()

	// Proxy Password Input
	if c.Proxy.Pass != "" {
		pwPrompt := "word:"
		idx, _ := child.ExpectTimeout(20*time.Second, regexp.MustCompile(pwPrompt))
		if idx >= 0 {
			child.SendLine(c.Proxy.Pass)

		} else {
			fmt.Println("Not Connected")
			return
		}
	}

	// Password Input
	if c.Pass != "" {
		pwPrompt := "word:"
		idx, _ := child.ExpectTimeout(20*time.Second, regexp.MustCompile(pwPrompt))
		if idx >= 0 {
			child.SendLine(c.Pass)

		} else {
			fmt.Println("Not Connected")
			return
		}
	}

	// timeout
	child.InteractTimeout(2419200 * time.Second)
	child.Close()
	return
}
