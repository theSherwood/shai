package bootstrap

import "embed"

//go:embed bootstrap.sh
var Script []byte

//go:embed shai-remote.sh
var AliasScript []byte

//go:embed conf/tinyproxy.conf conf/dnsmasq.conf conf/dnsmasq.d/*
var ConfFS embed.FS
