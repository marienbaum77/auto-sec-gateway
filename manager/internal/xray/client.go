package xray

import (
	"context"
	"fmt"
	"log"

	"github.com/xtls/xray-core/app/proxyman/command"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/vless"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	apiAddr string
}

func NewClient(addr string) *Client {
	return &Client{apiAddr: addr}
}

// AddUser добавляет UUID в работающий Xray без перезагрузки
func (c *Client) AddUser(ctx context.Context, email, uuid string, flow string) error {
	// 1. Устанавливаем соединение с gRPC сервером Xray
	conn, err := grpc.Dial(c.apiAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to dial xray api: %w", err)
	}
	defer conn.Close()

	client := command.NewHandlerServiceClient(conn)

	// 2. Формируем настройки аккаунта VLESS
	vlessAccount := &vless.Account{
		Id:   uuid,
		Flow: flow, // Обязательно передаем xtls-rprx-vision
	}

	// 3. Упаковываем специфичные для VLESS настройки в универсальный формат Xray
	accountAny := serial.ToTypedMessage(vlessAccount)

	// 4. Формируем структуру пользователя
	user := &protocol.User{
		Level:   0,
		Email:   email,
		Account: accountAny,
	}

	// 5. Упаковываем операцию "Добавить пользователя"
	operation := serial.ToTypedMessage(&command.AddUserOperation{
		User: user,
	})

	// 6. Отправляем команду в Inbound с тегом "proxy-in"
	_, err = client.AlterInbound(ctx, &command.AlterInboundRequest{
		Tag:       "proxy-in",
		Operation: operation,
	})

	if err != nil {
		return fmt.Errorf("failed to alter inbound: %w", err)
	}

	log.Printf("[gRPC] Пользователь %s (%s) успешно добавлен в память Xray", email, uuid)
	return nil
}