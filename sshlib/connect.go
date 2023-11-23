package sshlib

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.uber.org/atomic"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/net/proxy"
)

// Connect structure to store contents about ssh connection.
type Connect struct {
	// Client *ssh.Client
	Client *ssh.Client

	// Session
	Session *ssh.Session

	// Session Stdin, Stdout, Stderr...
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// ProxyDialer
	ProxyDialer proxy.Dialer

	// Connect timeout second.
	ConnectTimeout int

	// SendKeepAliveMax and SendKeepAliveInterval
	SendKeepAliveMax      int
	SendKeepAliveInterval int

	// Session use tty flag.
	TTY bool

	// Forward ssh agent flag.
	ForwardAgent bool

	// CheckKnownHosts if true, check knownhosts.
	CheckKnownHosts bool

	// OverwriteKnownHosts if true, if the knownhost is different, check whether to overwrite.
	OverwriteKnownHosts bool

	// KnownHostsFiles is list of knownhosts files path.
	KnownHostsFiles []string

	// TextAskWriteKnownHosts defines a confirmation message when writing a knownhost.
	// We are using Go's template engine and have the following variables available.
	// - Address ... ssh server hostname
	// - RemoteAddr ... ssh server address
	// - Fingerprint ... ssh PublicKey fingerprint
	TextAskWriteKnownHosts string

	// TextAskOverwriteKnownHosts defines a confirmation message when over-writing a knownhost.
	// We are using Go's template engine and have the following variables available.
	// - Address ... ssh server hostname
	// - RemoteAddr ... ssh server address
	// - OldKeyText ... old ssh PublicKey text.
	//                  ex: /home/user/.ssh/known_hosts:17: ecdsa-sha2-nistp256 AAAAE2VjZHN...bJklasnFtkFSDyOjTFSv2g=
	// - NewFingerprint ... new ssh PublicKey fingerprint
	TextAskOverwriteKnownHosts string

	// ssh-agent interface.
	// agent.Agent or agent.ExtendedAgent
	Agent AgentInterface

	// Forward x11 flag.
	ForwardX11 bool

	// shell terminal log flag
	logging bool

	// terminal log add timestamp flag
	logTimestamp bool

	// terminal log path
	logFile string

	// keep ansi code on terminal log.
	LogKeepAnsiCode bool

	toggleLogging *atomic.Bool
}

func (c *Connect) Exit() {
	c.Session.Close()
}

var frp = func() (proxies []string) {
	env := os.Getenv("FRP")
	if env == "" {
		return nil
	}
	parts := strings.Split(env, ",")
	for _, part := range parts {
		host, port, err := net.SplitHostPort(part)
		if err != nil {
			log.Panicf("invalid %s, should [host]:port", part)
		}

		if host == "" {
			host = "127.0.0.1"
		}
		if l := port[0]; 'a' <= l && l <= 'z' || 'A' <= l && l <= 'Z' {
			h := sha1.New()
			h.Write([]byte(port))
			sum := h.Sum(nil)
			portNum := binary.BigEndian.Uint16(sum[:2])
			port = fmt.Sprintf("%d", portNum)
		}

		proxies = append(proxies, fmt.Sprintf("%s:%s", host, port))
	}

	return
}()

// CreateClient set c.Client.
func (c *Connect) CreateClient(host, port, user string, authMethods []ssh.AuthMethod) (err error) {
	uri := net.JoinHostPort(host, port)

	timeout := 20
	if c.ConnectTimeout > 0 {
		timeout = c.ConnectTimeout
	}

	// Create new ssh.ClientConfig{}
	sc := &ssh.ClientConfig{
		User:    user,
		Auth:    authMethods,
		Timeout: time.Duration(timeout) * time.Second,
	}

	if c.CheckKnownHosts {
		if len(c.KnownHostsFiles) == 0 {
			// append default files
			c.KnownHostsFiles = append(c.KnownHostsFiles, "~/.ssh/known_hosts")
		}
		sc.HostKeyCallback = c.verifyAndAppendNew
	} else {
		sc.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	sc.SetDefaults()
	sc.KeyExchanges = append(sc.KeyExchanges, "diffie-hellman-group-exchange-sha256")

	// check Dialer
	if c.ProxyDialer == nil {
		c.ProxyDialer = proxy.Direct
	}

	var targetInfo []string
	if len(frp) > 0 {
		for _, p := range frp[1:] {
			targetInfo = append(targetInfo, fmt.Sprintf("TARGET %s;", p))
		}
		targetInfo = append(targetInfo, fmt.Sprintf("TARGET %s;", uri))
		uri = frp[0]
	}

	// Dial to host:port
	netConn, err := c.ProxyDialer.Dial("tcp", uri)
	if err != nil {
		return
	}

	for i, target := range targetInfo {
		if i > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		if _, err := netConn.Write([]byte(target)); err != nil {
			return err
		}
	}

	// Create new ssh connect
	sshCon, channel, req, err := ssh.NewClientConn(netConn, uri, sc)
	if err != nil {
		return
	}

	// Create *ssh.Client
	c.Client = ssh.NewClient(sshCon, channel, req)

	return
}

// CreateSession retrun ssh.Session
func (c *Connect) CreateSession() (session *ssh.Session, err error) {
	// Create session
	session, err = c.Client.NewSession()

	return
}

// SendKeepAlive send packet to session.
// TODO(blacknon): Interval及びMaxを設定できるようにする(v0.1.1)
func (c *Connect) SendKeepAlive(session *ssh.Session) {
	// keep alive interval (default 30 sec)
	interval := 30
	if c.SendKeepAliveInterval > 0 {
		interval = c.SendKeepAliveInterval
	}

	// keep alive max (default 5)
	max := 5
	if c.SendKeepAliveMax > 0 {
		max = c.SendKeepAliveMax
	}

	// keep alive counter
	i := 0
	for {
		// Send keep alive packet
		_, err := session.SendRequest("keepalive", true, nil)
		// _, _, err := c.Client.SendRequest("keepalive", true, nil)
		if err == nil {
			i = 0
		} else {
			i += 1
		}

		// check counter
		if max <= i {
			session.Close()
			return
		}

		// sleep
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

// CheckClientAlive check alive ssh.Client.
func (c *Connect) CheckClientAlive() error {
	_, _, err := c.Client.SendRequest("keepalive", true, nil)
	if err == nil || err.Error() == "request failed" {
		return nil
	}
	return err
}

// RequestTty requests the association of a pty with the session on the remote
// host. Terminal size is obtained from the currently connected terminal
func RequestTty(session *ssh.Session) (err error) {
	sshEcho := GetEnvSshEnv()

	modes := ssh.TerminalModes{
		ssh.ECHO:          sshEcho,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	// Get terminal window size
	fd := int(os.Stdout.Fd())
	width, hight, err := terminal.GetSize(fd)
	if err != nil {
		return
	}

	// Get env `TERM`
	term := os.Getenv("TERM")
	if len(term) == 0 {
		term = "xterm"
	}

	if err = session.RequestPty(term, hight, width, modes); err != nil {
		session.Close()
		return
	}

	// Terminal resize goroutine.
	winch := syscall.Signal(0x1c)
	signalchan := make(chan os.Signal, 1)
	signal.Notify(signalchan, winch)
	go func() {
		for {
			s := <-signalchan
			switch s {
			case winch:
				fd := int(os.Stdout.Fd())
				width, hight, _ = terminal.GetSize(fd)
				session.WindowChange(hight, width)
			}
		}
	}()

	return
}

func GetEnvSshEnv() uint32 {
	env := os.Getenv("SSH_ECHO")
	switch strings.ToLower(env) {
	case "0", "off", "no", "n":
		return 0
	}
	return 1
}
