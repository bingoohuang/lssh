package sshlib

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/bingoohuang/bssh/internal/util"
	"github.com/bingoohuang/ngg/gossh/pkg/gossh"
	"github.com/bingoohuang/ngg/ss"
	"github.com/bingoohuang/ngg/tsid"
	"golang.org/x/term"
)

func (c *Connect) interruptInput(webPort int, hostInfoScript string, hostInfoUpdater func(hostInfo string), shellReader func(p []byte), processInfoScript string) (
	stdin *io.PipeReader, stdout *io.PipeWriter, pipeToStdin *io.PipeWriter, ir *interruptReader) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	notifyC := make(chan NotifyCmd)
	notifyRspC := make(chan string)

	iw := newInterruptWriter(r2, notifyC, notifyRspC, shellReader)
	go func() {
		if _, err := io.Copy(os.Stdout, iw); err != nil && errors.Is(err, io.EOF) {
			return
		}
	}()

	ir = newInterruptReader(webPort, notifyC, notifyRspC, w1, c, hostInfoScript, hostInfoUpdater, processInfoScript)
	go func() {
		if _, err := io.Copy(w1, ir); err != nil && errors.Is(err, io.EOF) {
			return
		}
	}()

	return r1, w2, w1, ir
}

func newInterruptReader(port int, notifyC chan NotifyCmd, notifyRspC chan string,
	directWriter *io.PipeWriter, connect *Connect, hostInfoScript string, hostInfoUpdater func(hostInfo string), processInfoScript string) *interruptReader {
	return &interruptReader{
		r:                 GetStdin(),
		port:              port,
		directWriter:      directWriter,
		notifyC:           notifyC,
		notifyRspC:        notifyRspC,
		connect:           connect,
		hostInfoScript:    hostInfoScript,
		hostInfoUpdater:   hostInfoUpdater,
		processInfoScript: processInfoScript,
	}
}

func newInterruptWriter(r io.Reader, notifyC chan NotifyCmd, notifyRspC chan string, shellReader func(p []byte)) io.Reader {
	return &interruptWriter{
		r:           r,
		notifyC:     notifyC,
		notifyRspC:  notifyRspC,
		shellReader: shellReader,
	}
}

type interruptWriter struct {
	r           io.Reader
	notifyC     chan NotifyCmd
	notifyTag   string
	buf         bytes.Buffer
	notifyRspC  chan string
	notifyTime  time.Time
	shellReader func(p []byte)
}

func (i *interruptWriter) Read(p []byte) (n int, err error) {
	n, err = i.r.Read(p)
	if n == 0 || err != nil {
		return 0, err
	}

	if i.shellReader != nil {
		i.shellReader(p[:n])
	}

	if i.notifyTag != "" && time.Since(i.notifyTime) < 15*time.Second {
		i.buf.Write(p[:n])
		if bytes.Contains(i.buf.Bytes(), []byte("close:"+i.notifyTag+"\r\n")) {
			rsp, closeFound := clearTag(i.notifyTag, i.buf.Bytes())
			if closeFound {
				i.notifyRspC <- rsp
				i.buf.Reset()
				i.notifyTag = ""
			}
		}

		return 0, nil
	}

	select {
	case notify := <-i.notifyC:
		i.notifyTag = notify.Value
		i.notifyTime = time.Now()
		i.buf.Reset()
		i.buf.Write(p[:n])
		return 0, nil
	default:
	}

	return n, nil
}

func clearTag(tag string, b []byte) (string, bool) {
	openTag := []byte("open:" + tag + "\r\n")
	openPos := bytes.Index(b, openTag)
	if openPos < 0 {
		return "", false
	}

	closeTag := []byte("close:" + tag)
	closePos := bytes.Index(b[openPos:], closeTag)
	if closePos < 0 {
		return "", false
	}

	s := string(b[openPos+len(openTag) : openPos+closePos])
	return strings.TrimSpace(s), true
}

type NotifyType int

const (
	NotifyTypeTag NotifyType = iota
)

type NotifyCmd struct {
	Type  NotifyType
	Value string
}

type interruptReader struct {
	r            io.Reader
	port         int
	directWriter *io.PipeWriter
	notifyC      chan NotifyCmd
	notifyRspC   chan string
	connect      *Connect

	LastKeyCtrK       bool
	LastKeyCtrKTime   time.Time
	hostInfoScript    string
	hostInfoUpdater   func(hostInfo string)
	processInfoScript string
}

func (i *interruptReader) Read(p []byte) (n int, err error) {
	if GetEnvSshEnv() == 1 {
		n, err = i.r.Read(p)
		if n == 0 {
			return 0, err
		}

		isKeyCtrK := n == 1 && p[0] == gossh.KeyCtrlK
		now := time.Now()
		defer func() {
			i.LastKeyCtrK = isKeyCtrK
			i.LastKeyCtrKTime = now
		}()
		if !isKeyCtrK || !i.LastKeyCtrK || now.Sub(i.LastKeyCtrKTime) > time.Second {
			return n, nil
		}
		_, _ = os.Stdout.Write([]byte(">> "))
		if i.port > 0 {
			i.openWebExplorer()
		}
	}

Next:
	screen := struct {
		io.Reader
		io.Writer
	}{Reader: os.Stdin, Writer: os.Stdout}
	t := term.NewTerminal(screen, "")
	line, err := t.ReadLine()
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = nil
		}
		_, _ = i.directWriter.Write([]byte("\r"))
		return 0, err
	}

	cmdFields := strings.Fields(line)
	i.connect.ToggleLogging(false)
	defer i.connect.ToggleLogging(true)

	cmd := ss.If(len(cmdFields) > 0, strings.ToLower(cmdFields[0]), "")

	if len(cmdFields) == 1 && ss.AnyOf(cmd, ".?") {
		fmt.Print("Available commands:\r\n"+
			"0) .?            : to show help info\r\n"+
			"1) .dash         : to open the info page in browser\r\n"+
			"2) .web          : to open the file explorer in browser\r\n"+
			"3) .up localfile : to upload the local file to the remote\r\n"+
			"4) .dl remotefile: to download the remote file to the local\r\n",
			"5) .hostinfo     : to show host info\r\n",
			"6) .exit         : to exit the current bssh connection\r\n",
			"7) .ps {pid}     : to print process info\r\n",
		)
	} else if len(cmdFields) == 1 && ss.AnyOf(cmd, ".hostinfo") {
		if i.hostInfoScript == "" {
			log.Printf("hostInfoScript is empty")
		} else {
			hostInfo, err := i.executeCmd(i.hostInfoScript, 15*time.Second)
			if err != nil {
				log.Printf("host info error: %v", err)
			} else {
				hostInfo = regexp.MustCompile(`[\r\n]+`).ReplaceAllString(hostInfo, "")
				fmt.Printf("主机信息: %s\n", hostInfo)
				if i.hostInfoUpdater != nil {
					i.hostInfoUpdater(hostInfo)
				}
			}
		}
	} else if len(cmdFields) == 2 && ss.AnyOf(cmd, ".ps") {
		if i.processInfoScript == "" {
			log.Printf("processInfoScript is empty")
		} else {
			t, err1 := template.New("ps").Parse(i.processInfoScript)
			if err1 != nil {
				log.Printf("E! parse processInfoScript error: %v", err1)
				return 0, err
			}

			sep := tsid.Fast().ToString()
			var b bytes.Buffer
			if err1 := t.Execute(&b, map[string]any{
				"Pid":       cmdFields[1],
				"NewLine":   sep,
				"LocalTime": time.Now().Format("2006-01-02T15:04:05Z0700"),
			}); err1 != nil {
				log.Printf("E! execute processInfoScript error: %v", err1)
				return 0, err
			}
			for _, cmd := range ss.Split(b.String(), sep) {
				i.directWriter.Write([]byte(cmd + "\r\r"))
				time.Sleep(time.Millisecond * 500)
			}
		}
	} else if len(cmdFields) == 1 && ss.AnyOf(cmd, ".dash") {
		if i.port > 0 {
			go util.OpenBrowser(fmt.Sprintf("http://127.0.0.1:%d/dash", i.port))
		} else {
			fmt.Print("dash is not available\r\n")
		}
	} else if len(cmdFields) == 1 && ss.AnyOf(cmd, ".web") {
		if i.port > 0 {
			i.openWebExplorer()
		} else {
			fmt.Print("dash is not available\r\n")
		}
	} else if len(cmdFields) == 1 && ss.AnyOf(cmd, ".exit", ".quit") {
		i.directWriter.Write([]byte("exit"))
	} else if len(cmdFields) == 2 && ss.AnyOf(cmd, ".up") {
		i.up(cmdFields[1])
	} else if len(cmdFields) == 2 && ss.AnyOf(cmd, ".dl") {
		i.dl(cmdFields[1])

		// 参考 https://github.com/M09Ic/rscp
		// 		if opt.upload blockSize = 20480
		//		if opt.download  blockSize = 102400
		// 下载 cmd := fmt.Sprintf("dd if=%s bs=%d count=1 skip=%d 2>/dev/null | base64 -w 0 && echo", remotefile, blockSize, off)
		// 上传 cmd := fmt.Sprintf("echo %s | base64 -d > %s && md5sum %s", content, tmpfile, tmpfile)
		// 合并文件: cd %s && cat %s > %s
	} else {
		i.directWriter.Write([]byte(line))
	}

	i.directWriter.Write([]byte("\r"))
	if GetEnvSshEnv() == 0 {
		goto Next
	}
	return 0, err
}

func (i *interruptReader) openWebExplorer() {
	// TODO: support current directory on d5k
	// pwd := i.executeCmd("pwd")
	pwd := ""
	// http://127.0.0.1:8333/files/home/footstone/
	go util.OpenBrowser(fmt.Sprintf("http://127.0.0.1:%d/files%s", i.port, pwd))
}
