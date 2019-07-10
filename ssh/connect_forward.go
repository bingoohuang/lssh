package ssh

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/blacknon/lssh/common"
	"golang.org/x/crypto/ssh"
)

// TODO(blacknon):
//     socket forwardについても実装する

type x11request struct {
	SingleConnection bool
	AuthProtocol     string
	AuthCookie       string
	ScreenNumber     uint32
}

func x11SocketForward(channel ssh.Channel) {
	// TODO(blacknon): Socket通信しか考慮されていないので、TCP通信での指定もできるようにする
	conn, err := net.Dial("unix", os.Getenv("DISPLAY"))
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		io.Copy(conn, channel)
		conn.(*net.UnixConn).CloseWrite()
		wg.Done()
	}()
	go func() {
		io.Copy(channel, conn)
		channel.CloseWrite()
		wg.Done()
	}()

	wg.Wait()
	conn.Close()
	channel.Close()
}

func (c *Connect) X11Forwarder(session *ssh.Session) {
	// set x11-req Payload
	payload := x11request{
		SingleConnection: false,
		AuthProtocol:     string("MIT-MAGIC-COOKIE-1"),
		AuthCookie:       string(common.NewSHA1Hash()),
		ScreenNumber:     uint32(0),
	}

	// Send x11-req Request
	ok, err := session.SendRequest("x11-req", true, ssh.Marshal(payload))
	if err == nil && !ok {
		fmt.Println(errors.New("ssh: x11-req failed"))
	} else {
		// Open HandleChannel x11
		x11channels := c.Client.HandleChannelOpen("x11")

		go func() {
			for ch := range x11channels {
				channel, _, err := ch.Accept()
				if err != nil {
					continue
				}

				go x11SocketForward(channel)
			}
		}()
	}
}

// forward function to do port io.Copy with goroutine
func (c *Connect) portForward(localConn net.Conn) {
	// TODO(blacknon): 関数名等をちゃんと考える

	// Create ssh connect
	sshConn, err := c.Client.Dial("tcp", c.ForwardRemote)

	// Copy localConn.Reader to sshConn.Writer
	go func() {
		_, err = io.Copy(sshConn, localConn)
		if err != nil {
			fmt.Printf("Port forward local to remote failed: %v\n", err)
		}
	}()

	// Copy sshConn.Reader to localConn.Writer
	go func() {
		_, err = io.Copy(localConn, sshConn)
		if err != nil {
			fmt.Printf("Port forward remote to local failed: %v\n", err)
		}
	}()
}

// PortForwarder port forwarding based on the value of Connect
func (c *Connect) PortForwarder() {
	// TODO(blacknon):
	// 現在の方式だと、クライアント側で無理やりポートフォワーディングをしている状態なので、RFCに沿ってport forwardさせる処理についても追加する
	//
	// 【参考】
	//     - https://github.com/maxhawkins/easyssh/blob/a4ce364b6dd8bf2433a0d67ae76cf1d880c71d75/tcpip.go
	//     - https://www.unixuser.org/~haruyama/RFC/ssh/rfc4254.txt
	//
	// TODO(blacknon): 関数名等をちゃんと考える

	// Open local port.
	localListener, err := net.Listen("tcp", c.ForwardLocal)

	if err != nil {
		// error local port open.
		fmt.Fprintf(os.Stdout, "local port listen failed: %v\n", err)
	} else {
		// start port forwarding.
		go func() {
			for {
				// Setup localConn (type net.Conn)
				localConn, err := localListener.Accept()
				if err != nil {
					fmt.Printf("listen.Accept failed: %v\n", err)
				}
				go c.portForward(localConn)
			}
		}()
	}
}