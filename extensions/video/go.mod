module github.com/miguelemosreverte/family-vpn/extensions/video

go 1.25.4

require (
	github.com/miguelemosreverte/family-vpn/extensions/framework v0.0.0
	github.com/miguelemosreverte/family-vpn/ipc v0.0.0
	github.com/miguelemosreverte/family-vpn/video-call v0.0.0
)

require github.com/gorilla/websocket v1.5.3 // indirect

replace (
	github.com/miguelemosreverte/family-vpn/extensions/framework => ../framework
	github.com/miguelemosreverte/family-vpn/ipc => ../../ipc
	github.com/miguelemosreverte/family-vpn/video-call => ../../video-call
)
