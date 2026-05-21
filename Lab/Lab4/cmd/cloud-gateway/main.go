package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"battleworld/cloudapi"
	"battleworld/protocol"
)

func main() {
	gatewayAddr := defaultEnv("LAB4_GATEWAY_ADDR", "0.0.0.0:9310")
	coordURL := cloudapi.NormalizeBaseURL(requiredEnv("LAB4_COORDINATOR_URL"))

	ln, err := net.Listen("tcp", gatewayAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "网关监听失败：%v\n", err)
		os.Exit(1)
	}
	defer ln.Close()

	fmt.Printf("云网关已启动：listen=%s coordinator=%s\n", gatewayAddr, coordURL)
	for {
		raw, err := ln.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "接入连接失败：%v\n", err)
			continue
		}
		go handleClient(raw, coordURL)
	}
}

func handleClient(raw net.Conn, coordURL string) {
	conn := protocol.NewConn(raw)
	defer conn.Close()

	authMsg, err := conn.Receive()
	if err != nil {
		return
	}
	client := cloudapi.NewHTTPClient()

	if authMsg.Type == protocol.TypeAdmin {
		resp, err := callCoordinator(client, coordURL, cloudapi.CoordinatorRequest{
			Action:      cloudapi.CoordinatorActionAdmin,
			AdminAction: authMsg.Action,
			NodeID:      authMsg.NodeID,
		})
		if err != nil {
			_ = conn.Send(protocol.Message{Type: protocol.TypeError, Error: err.Error()})
			return
		}
		_ = conn.Send(protocol.Message{Type: protocol.TypeAdmin, OK: true, Text: resp.Text})
		return
	}

	resp, err := callCoordinator(client, coordURL, cloudapi.CoordinatorRequest{
		Action:   authMsg.Type,
		Username: authMsg.Username,
		Password: authMsg.Password,
		Confirm:  authMsg.Confirm,
	})
	if err != nil {
		_ = conn.Send(protocol.Message{Type: protocol.TypeError, Error: err.Error()})
		return
	}
	username := authMsg.Username
	if err := conn.Send(protocol.Message{Type: protocol.TypeAuth, OK: true, State: resp.State}); err != nil {
		_, _ = callCoordinator(client, coordURL, cloudapi.CoordinatorRequest{Action: cloudapi.CoordinatorActionLogout, Username: username})
		return
	}

	var once sync.Once
	done := make(chan struct{})
	stop := func() {
		once.Do(func() {
			close(done)
			_, _ = callCoordinator(client, coordURL, cloudapi.CoordinatorRequest{Action: cloudapi.CoordinatorActionLogout, Username: username})
		})
	}
	defer stop()

	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				resp, err := callCoordinator(client, coordURL, cloudapi.CoordinatorRequest{
					Action:   cloudapi.CoordinatorActionSnapshot,
					Username: username,
				})
				if err != nil {
					_ = conn.Send(protocol.Message{Type: protocol.TypeError, Error: err.Error()})
					stop()
					return
				}
				if err := conn.Send(protocol.Message{Type: protocol.TypeState, State: resp.State}); err != nil {
					stop()
					return
				}
			case <-done:
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		default:
		}

		msg, err := conn.Receive()
		if err != nil {
			return
		}

		if msg.Type == protocol.TypeLogout {
			return
		}

		req := cloudapi.CoordinatorRequest{
			Action:   msg.Type,
			Username: username,
			Dir:      msg.Dir,
			MapID:    msg.MapID,
			Item:     msg.Item,
			Target:   msg.Target,
			Amount:   msg.Amount,
		}
		resp, err := callCoordinator(client, coordURL, req)
		if err != nil {
			if sendErr := conn.Send(protocol.Message{Type: protocol.TypeError, Error: err.Error()}); sendErr != nil {
				return
			}
			continue
		}
		if resp.State != nil {
			if err := conn.Send(protocol.Message{Type: protocol.TypeState, State: resp.State}); err != nil {
				return
			}
		}
	}
}

func callCoordinator(client *http.Client, coordURL string, req cloudapi.CoordinatorRequest) (cloudapi.CoordinatorResponse, error) {
	var resp cloudapi.CoordinatorResponse
	if err := cloudapi.PostJSON(client, coordURL+"/v1/coordinator", req, &resp); err != nil {
		return resp, err
	}
	if !resp.OK {
		return resp, fmt.Errorf(resp.Error)
	}
	return resp, nil
}

func requiredEnv(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		fmt.Fprintf(os.Stderr, "缺少环境变量 %s\n", key)
		os.Exit(1)
	}
	return value
}

func defaultEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
