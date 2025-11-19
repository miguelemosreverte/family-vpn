module github.com/miguelemosreverte/family-vpn/extensions/ssh

go 1.25.4

require (
	github.com/miguelemosreverte/family-vpn/extensions/framework v0.0.0
	github.com/miguelemosreverte/family-vpn/ipc v0.0.0
)

replace (
	github.com/miguelemosreverte/family-vpn/extensions/framework => ../framework
	github.com/miguelemosreverte/family-vpn/ipc => ../../ipc
)
