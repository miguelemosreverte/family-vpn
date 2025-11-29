module vpn-server

go 1.25.1

require (
	github.com/gorilla/websocket v1.5.3
	github.com/miguelemosreverte/family-vpn/protocol v0.0.0
	github.com/songgao/water v0.0.0-20200317203138-2b4b6d7c09d8
)

require golang.org/x/sys v0.37.0 // indirect

replace github.com/miguelemosreverte/family-vpn/protocol => ../protocol
