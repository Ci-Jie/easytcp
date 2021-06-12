package server

import (
	"github.com/DarthPestilane/easytcp/packet"
	"github.com/DarthPestilane/easytcp/router"
	"github.com/stretchr/testify/assert"
	"net"
	"runtime"
	"testing"
	"time"
)

func TestTCPServer_Serve(t *testing.T) {
	goroutineNum := runtime.NumGoroutine()
	server := NewTCPServer(TCPOption{})
	go func() {
		err := server.Serve("localhost:0")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "accept err")
	}()
	<-server.accepting
	err := server.Stop()
	assert.NoError(t, err)
	<-time.After(time.Millisecond * 10)
	assert.Equal(t, goroutineNum, runtime.NumGoroutine()) // no goroutine leak
}

func TestTCPServer_acceptLoop(t *testing.T) {
	server := NewTCPServer(TCPOption{
		RWBufferSize: 1024,
	})
	address, err := net.ResolveTCPAddr("tcp", "localhost:0")
	assert.NoError(t, err)
	lis, err := net.ListenTCP("tcp", address)
	assert.NoError(t, err)
	server.listener = lis
	go func() {
		err := server.acceptLoop()
		assert.Error(t, err)
	}()

	<-server.accepting

	// client
	cli, err := net.Dial("tcp", lis.Addr().String())
	assert.NoError(t, err)
	assert.NoError(t, cli.Close())
	assert.NoError(t, server.Stop())
}

func TestTCPServer_Stop(t *testing.T) {
	server := NewTCPServer(TCPOption{})
	go func() {
		err := server.Serve("localhost:0")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "accept err")
	}()

	<-server.accepting

	// client
	cli, err := net.Dial("tcp", server.listener.Addr().String())
	assert.NoError(t, err)

	<-time.After(time.Millisecond * 10)

	assert.NoError(t, server.Stop()) // stop server first
	assert.NoError(t, cli.Close())
}

func TestTCPServer_handleConn(t *testing.T) {
	type TestReq struct {
		Param string
	}
	type TestResp struct {
		Success bool
	}

	// options
	codec := &packet.JsonCodec{}
	packer := &packet.DefaultPacker{}

	// server
	server := NewTCPServer(TCPOption{
		RWBufferSize: 1024,
		MsgCodec:     codec,
		MsgPacker:    packer,
	})

	// register route
	server.AddRoute(1, func(ctx *router.Context) (packet.Message, error) {
		var reqData TestReq
		assert.NoError(t, ctx.Bind(&reqData))
		assert.EqualValues(t, 1, ctx.MsgID())
		assert.Equal(t, reqData.Param, "hello test")
		return ctx.Response(2, &TestResp{Success: true})
	})
	// use middleware
	server.Use(func(next router.HandlerFunc) router.HandlerFunc {
		return func(ctx *router.Context) (packet.Message, error) {
			defer func() {
				if r := recover(); r != nil {
					assert.Fail(t, "caught panic")
				}
			}()
			return next(ctx)
		}
	})

	go func() {
		err := server.Serve("localhost:0")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "accept err")
	}()
	defer func() { assert.NoError(t, server.Stop()) }()

	<-server.accepting

	// client
	cli, err := net.Dial("tcp", server.listener.Addr().String())
	assert.NoError(t, err)
	defer func() { assert.NoError(t, cli.Close()) }()

	// client send msg
	reqData := &TestReq{Param: "hello test"}
	reqDataByte, err := codec.Encode(reqData)
	assert.NoError(t, err)
	msg := &packet.DefaultMsg{
		ID:   1,
		Size: uint32(len(reqDataByte)),
		Data: reqDataByte,
	}
	reqMsg, err := packer.Pack(msg)
	assert.NoError(t, err)
	_, err = cli.Write(reqMsg)
	assert.NoError(t, err)

	// client read msg
	respMsg, err := packer.Unpack(cli)
	assert.NoError(t, err)
	var respData TestResp
	assert.NoError(t, codec.Decode(respMsg.GetData(), &respData))
	assert.EqualValues(t, 2, respMsg.GetID())
	assert.True(t, respData.Success)
}
