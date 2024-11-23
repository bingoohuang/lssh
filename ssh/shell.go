package ssh

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bingoohuang/bssh/common"
	"github.com/bingoohuang/bssh/conf"
	"github.com/bingoohuang/bssh/internal/stash"
	"github.com/bingoohuang/bssh/misc"
	"github.com/bingoohuang/bssh/sshlib"
	"github.com/bingoohuang/ngg/gossh/pkg/hostparse"
	"github.com/bingoohuang/ngg/ss"
	"github.com/bingoohuang/ngg/tsid"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// run shell

func (r *Run) shell() (err error) {
	server := r.ServerList[0]
	config, ok := r.Conf.Server[server]
	isTempHost := !ok
	if isTempHost {
		config = r.parseDirectServer(server)
	}

	// check count AuthMethod
	if len(r.serverAuthMethodMap[server]) == 0 {
		msg := fmt.Sprintf("Error: %s has No AuthMethod.\n", server)

		return errors.New(msg)
	}

	r.overwritePortForwardConfig(&config)
	r.overwriteBashrcConfig(&config)

	// header
	r.PrintSelectServer()
	r.printPortForward(config.PortForwardMode, config.PortForwardLocal, config.PortForwardRemote)
	r.printDynamicPortForward(config.DynamicPortForward)
	r.printProxy(server)

	if config.LocalRcUse == misc.Yes {
		fmt.Fprintf(os.Stderr, "Information   :This connect use local bashrc.\n")
	}

	// Create sshlib.Connect (Connect Proxy loop)
	connect, err := r.CreateSSHConnect(server)
	if err != nil {
		return err
	}

	if yes, _ := ss.GetenvBool("STASH", false); yes {
		if config.WebPort <= 0 {
			config.WebPort = ss.Rand().Port(8333)
		}
	}
	if config.WebPort > 0 {
		r.webPort, err = stash.InitFileStash(config.WebPort, connect, execCmd, SftpUpload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "InitFileStash error %s.\n", err.Error())
		}
	}

	// Create session
	session, err := connect.CreateSession()
	if err != nil {
		return err
	}

	if isTempHost {
		r.Conf.WriteTempHosts(server, config.Pass)
	}

	r.sshAgent(&config, connect, session)

	err = r.portForwarding(&config, connect)

	if config.DynamicPortForward != "" { // Dynamic Port Forwarding
		go func() {
			if err := connect.TCPDynamicForward("localhost", config.DynamicPortForward); err != nil {
				fmt.Println(err)
			}
		}()
	}

	// switch check Not-execute flag
	// TDXX(blacknon): Backgroundフラグを実装したら追加
	switch {
	case r.IsNone:
		r.noneExecute()

	default:
		// run pre local command
		if config.PreCmd != "" {
			execLocalCommand(config.PreCmd)
		}

		// defer run post local command
		if config.PostCmd != "" {
			defer execLocalCommand(config.PostCmd)
		}

		// if terminal log enable
		logConf := r.Conf.Log
		if logConf.Enable {
			logPath := r.getLogPath(server)
			fmt.Printf("logging to %s\n", logPath)
			connect.SetLog(logPath, logConf.Timestamp)
		}

		// TDXX(blacknon): local rc file add
		if config.LocalRcUse == misc.Yes {
			err = localrcShell(connect, session, config.LocalRcPath, config.LocalRcDecodeCmd)
		} else {
			hostInfoScript := defaultHostInfoScript
			hostInfoAutoEnabled := r.Conf.HostInfoEnabled.Get()

			scriptFile := r.Conf.HostInfoScriptFile
			if scriptFile != "" {
				if !strings.HasPrefix(scriptFile, "/") {
					scriptFile = filepath.Clean(filepath.Join(filepath.Dir(r.Conf.ConfPath), scriptFile))
				}
				script, err := os.ReadFile(scriptFile)
				if err != nil {
					log.Fatalf("read %q error: %v", scriptFile, err)
				}
				hostInfoScript = string(script)

			}
			hostInfoScript = strings.TrimRight(strings.TrimSpace(hostInfoScript), ";")
			hostInfoScript = regexp.MustCompile(`[\r\n]+`).ReplaceAllString(hostInfoScript, "")

			err = connect.ShellInitial(session, ConvertKeys(config.InitialCmd), config.InitialCmdSleep.Duration,
				r.webPort, hostInfoAutoEnabled, hostInfoScript,
				func(hostInfo string) {
					if r.Conf.HostInfo[server] == hostInfo {
						return
					}

					r.Conf.HostInfo[server] = hostInfo
					hostInfoJson, _ := json.Marshal(r.Conf.HostInfo)
					if len(hostInfoJson) > 0 {
						if err := os.WriteFile(r.Conf.HostInfoJsonFile, hostInfoJson, os.ModePerm); err != nil {
							log.Printf("write %q error: %v", r.Conf.HostInfoJsonFile, err)
						}
					}
				})
		}
	}

	return err
}

const defaultHostInfoScript = `uname -m; ` +
	`echo -n ", "; grep -c ^processor /proc/cpuinfo;` +
	`echo -n "C, "; free -h | awk '/^Mem:/ {print $7}';` +
	`echo -n "/"; free -h | awk '/^Mem:/ {print $2}';` +
	`echo -n ", "; df -h --total / | grep total | awk '{print $4}';` +
	`echo -n "/"; df -h --total / | grep total | awk '{print $2}';` +
	`echo -n ", "; lscpu | grep -E "型号名称" | awk -F '：' '{print $2}' | sed 's/^\s*//';` +
	`lscpu | grep -E "^Model name" | awk -F ':' '{print $2}' | sed 's/^\s*//';` +
	`echo -n ", "; cat /etc/os-release | grep ^PRETTY_NAME= | cut -d '"' -f2;`

func execCmd(connect *sshlib.Connect, cmd string) ([]byte, error) {
	session, err := connect.CreateSession()
	if err != nil {
		return nil, err
	}

	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout, session.Stderr = &stdoutBuf, &stderrBuf
	if err := session.Run(cmd); err != nil {
		return nil, err
	}

	//if stderr := stderrBuf.String(); stderr != "" {
	//	log.Printf("Exec cmd %s error: %s", cmd, stderr)
	//}

	return stdoutBuf.Bytes(), nil
}

func SftpUpload(client *sftp.Client, remote string, data []byte) error {
	// create destination file
	dstFile, err := client.Create(remote)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// copy source file to destination file
	if _, err := io.Copy(dstFile, bytes.NewReader(data)); err != nil {
		return err
	}

	return nil
}

func (r *Run) parseDirectServer(server string) (cf conf.ServerConfig) {
	sc, ok := hostparse.ParseDirectServer(server)
	if ok {
		autoID := "host-" + tsid.Fast().ToString()
		r.Conf.Server[autoID] = conf.ServerConfig{
			User: sc.User,
			Pass: sc.Pass,
			Addr: sc.Addr,
			Port: sc.Port,

			ID:           autoID,
			DirectServer: true,
		}
		r.registerAuthMapPassword(autoID, sc.Pass, "")
	}

	return r.Conf.Server[server]
}

func (r *Run) sshAgent(config *conf.ServerConfig, connect *sshlib.Connect, session *ssh.Session) {
	// ssh-agent
	if config.SSHAgentUse {
		connect.Agent = r.agent
		connect.ForwardSshAgent(session)
	}
}

func (r *Run) overwriteBashrcConfig(config *conf.ServerConfig) {
	// OverWrite local bashrc use
	if r.IsBashrc {
		config.LocalRcUse = misc.Yes
	}

	// OverWrite local bashrc not use
	if r.IsNotBashrc {
		config.LocalRcUse = "no"
	}
}

func (r *Run) overwritePortForwardConfig(config *conf.ServerConfig) {
	// OverWrite port forward mode
	if r.PortForwardMode != "" {
		config.PortForwardMode = r.PortForwardMode
	}

	// OverWrite port forwarding address
	if r.PortForwardLocal != "" && r.PortForwardRemote != "" {
		config.PortForwardLocal = r.PortForwardLocal
		config.PortForwardRemote = r.PortForwardRemote
	}

	// OverWrite dynamic port forwarding
	if r.DynamicPortForward != "" {
		config.DynamicPortForward = r.DynamicPortForward
	}
}

func (r *Run) portForwarding(config *conf.ServerConfig, connect *sshlib.Connect) (err error) {
	// Local/Remote Port Forwarding
	if config.PortForwardLocal != "" && config.PortForwardRemote != "" {
		// port forwarding
		switch config.PortForwardMode {
		case "L", "":
			err = connect.TCPLocalForward(config.PortForwardLocal, config.PortForwardRemote)
		case "R":
			err = connect.TCPRemoteForward(config.PortForwardLocal, config.PortForwardRemote)
		}

		if err != nil {
			fmt.Println(err)
		}
	}

	return err
}

// getLogPath return log file path.
func (r *Run) getLogPath(server string) (logPath string) {
	if idx := strings.Index(server, "@"); idx >= 0 {
		server = server[idx+1:]
	}

	server = strings.ReplaceAll(server, ":", "_")
	dir, dateFound, serverFound, err := r.getLogDirPath(server)
	if err != nil {
		log.Println(err)
	}

	var file string

	if !dateFound {
		file = time.Now().Format("20060102")
	}

	if !serverFound {
		if file != "" {
			file += "_"
		}
		file += server
	}

	file += ".log"
	logPath = filepath.Join(dir, file)

	return logPath
}

// getLogDirPath return log directory path.
func (r *Run) getLogDirPath(server string) (dir string, dateFound, hostnameFound bool, err error) {
	logConf := r.Conf.Log

	// expansion variable
	dir = ss.ExpandHome(logConf.Dir)
	dir, dateFound = Replace(dir, "<Date>", time.Now().Format("20060102"), 1)
	dir, hostnameFound = Replace(dir, "<ServerName>", server, 1)

	// create directory
	err = os.MkdirAll(dir, 0o700)

	return
}

func Replace(s, old, new string, n int) (r string, found bool) {
	r = strings.Replace(s, old, new, n)
	return r, r != s
}

// runLocalRcShell connect to remote shell using local bashrc.
func localrcShell(connect *sshlib.Connect, session *ssh.Session, localrcPath []string, decoder string) (err error) {
	// set default bashrc
	if len(localrcPath) == 0 {
		localrcPath = []string{"~/.bashrc"}
	}

	// get bashrc base64 data
	rcData, err := common.GetFilesBase64(localrcPath)
	if err != nil {
		return err
	}

	// command
	s := "bash --noprofile --rcfile<(echo %s|((base64 --help|grep -q coreutils)&&base64 -d<(cat)||base64 -D<(cat) ))"
	cmd := fmt.Sprintf(s, rcData)

	// decode command
	if decoder != "" {
		cmd = fmt.Sprintf("bash --noprofile --rcfile <(echo %s | %s)", rcData, decoder)
	}

	return connect.CmdShell(session, cmd)
}

// noneExecute is not execute command and shell.
func (r *Run) noneExecute() {
	for range time.After(500 * time.Millisecond) {
	}
}
