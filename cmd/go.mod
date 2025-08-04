module github.com/matteoavallone7/optimaLDN/cmd

go 1.23.3

require github.com/matteoavallone7/optimaLDN/src/common v0.0.0

require github.com/gorilla/websocket v1.5.3 // indirect

replace github.com/matteoavallone7/optimaLDN/src/common => ../src/common
