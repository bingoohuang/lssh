package ssh

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/blacknon/go-sshlib"
)

var cmdOPROMPT = "${SERVER} :: "

// cmd
// TODO(blacknon): Outputの出力処理について、Writerを用いた処理方法に切り替える
// TODO(blacknon): コマンドの実行処理を、sshlibをからの実行ではなく直接行わせる
func (r *Run) cmd() (err error) {
	// command
	command := strings.Join(r.ExecCmd, " ")

	// connect map
	connmap := map[string]*sshlib.Connect{}

	// make channel
	finished := make(chan bool)
	input := make(chan io.WriteCloser)
	exitInput := make(chan bool)

	// print header
	r.printSelectServer()
	r.printRunCommand()
	if len(r.ServerList) == 1 {
		r.printProxy(r.ServerList[0])
	}

	// Create sshlib.Connect loop
	for _, server := range r.ServerList {
		// check count AuthMethod
		if len(r.serverAuthMethodMap[server]) == 0 {
			fmt.Fprintf(os.Stderr, "Error: %s is No AuthMethod.\n", server)
			continue
		}

		// Create sshlib.Connect
		conn, err := r.createSshConnect(server)
		if err != nil {
			log.Printf("Error: %s:%s\n", server, err)
			continue
		}

		// stdin data check
		if len(r.stdinData) > 0 {
			conn.Stdin = r.stdinData
		}

		connmap[server] = conn
	}

	// Run command and print loop
	for s, fc := range connmap {
		// set variable c
		// NOTE: Variables need to be assigned separately for processing by goroutine.
		c := fc

		// Get server config
		config := r.Conf.Server[s]

		// create Output
		o := &Output{
			Templete:   cmdOPROMPT,
			Count:      0,
			ServerList: r.ServerList,
			Conf:       r.Conf.Server[s],
			AutoColor:  true,
		}
		o.Create(s)

		// create channel
		output := make(chan []byte)

		// Overwrite port forwarding.
		// Valid only when one server is specified
		if len(r.ServerList) == 1 {
			// if select server is single, Force a value such as.
			//     - session.Stdout = os.Stdout
			//     - session.Stderr = os.Stderr
			if r.IsTerm {
				c.ForceStd = true
			}

			// OverWrite port forward mode
			if r.PortForwardMode != "" {
				config.PortForwardMode = r.PortForwardMode
			}

			// Overwrite port forward address
			if r.PortForwardLocal != "" && r.PortForwardRemote != "" {
				config.PortForwardLocal = r.PortForwardLocal
				config.PortForwardRemote = r.PortForwardRemote
			}

			// print header
			r.printPortForward(config.PortForwardMode, config.PortForwardLocal, config.PortForwardRemote)

			// Port Forwarding
			switch config.PortForwardMode {
			case "L", "":
				c.TCPLocalForward(config.PortForwardLocal, config.PortForwardRemote)
			case "R":
				c.TCPRemoteForward(config.PortForwardLocal, config.PortForwardRemote)
			}

			// Dynamic Port Forwarding
			if config.DynamicPortForward != "" {
				r.printDynamicPortForward(config.DynamicPortForward)
				go c.TCPDynamicForward("localhost", config.DynamicPortForward)
			}
		}

		// run command
		// if parallel flag true, and select server is not single,
		// os.Stdin to multiple server.
		go func() {
			var err error
			if r.IsParallel && len(r.ServerList) > 1 {
				err = c.CmdWriter(command, output, input)
			} else {
				err = c.Cmd(command, output)
			}

			if err != nil {
				log.Println(err)
			}

			finished <- true
		}()

		if r.IsParallel {
			go printOutput(o, output)
		} else {
			printOutput(o, output)
		}

	}

	// if parallel flag true, and select server is not single,
	// create io.MultiWriter and send input.
	if r.IsParallel && len(r.ServerList) > 1 {
		writers := []io.WriteCloser{}
		for i := 0; i < len(r.ServerList); i++ {
			w := <-input
			writers = append(writers, w)
		}
		// writer := io.MultiWriter(writers...)
		go pushInput(exitInput, writers)
	}

	for i := 0; i < len(connmap); i++ {
		<-finished
	}

	close(exitInput)
	return
}
