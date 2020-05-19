module github.com/bingoohuang/bssh

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/VividCortex/ewma v1.1.1 // indirect
	github.com/acarl005/stripansi v0.0.0-20180116102854-5a71ef0e047d // indirect
	github.com/bingoohuang/gonet v0.0.0-20200224141149-35b28c91a637
	github.com/bingoohuang/gou v0.0.0-20200225004418-9b3655665c46
	github.com/blacknon/go-sshlib v0.1.1
	github.com/blacknon/textcol v0.0.1
	github.com/c-bata/go-prompt v0.2.3
	github.com/dustin/go-humanize v1.0.0
	github.com/jedib0t/go-pretty v4.3.0+incompatible
	github.com/kevinburke/ssh_config v0.0.0-20190724205821-6cfae18c12b8
	github.com/mattn/go-colorable v0.1.6 // indirect
	github.com/mattn/go-runewidth v0.0.7
	github.com/mattn/go-shellwords v1.0.6
	github.com/mattn/go-tty v0.0.0-20190424173100-523744f04859 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/nsf/termbox-go v0.0.0-20190325093121-288510b9734e
	github.com/pkg/sftp v1.11.1-0.20200310224833-18dc4db7a456
	github.com/pkg/term v0.0.0-20190109203006-aa71e9d9e942 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/pflag v1.0.3
	github.com/spf13/viper v1.6.1
	github.com/stretchr/testify v1.4.0
	github.com/thoas/go-funk v0.5.0
	github.com/urfave/cli v1.21.0
	github.com/vbauerster/mpb v3.4.0+incompatible
	github.com/youtube/vitess v2.1.1+incompatible // indirect
	golang.org/x/crypto v0.0.0-20200221231518-2aa609cf4a9d
	golang.org/x/net v0.0.0-20191209160850-c0dbc17a3553
	mvdan.cc/sh v2.6.3+incompatible
)

replace github.com/urfave/cli v1.22.0 => ../../urfave/cli

go 1.13
