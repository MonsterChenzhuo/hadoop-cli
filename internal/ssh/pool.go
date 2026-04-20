package ssh

import (
	"sync"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
)

type Pool struct {
	mu      sync.Mutex
	clients map[string]*Client
	inv     *inventory.Inventory
}

func NewPool(inv *inventory.Inventory) *Pool {
	return &Pool{clients: map[string]*Client{}, inv: inv}
}

func (p *Pool) Get(hostName string) (*Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[hostName]; ok {
		return c, nil
	}
	h, ok := p.inv.HostByName(hostName)
	if !ok {
		return nil, &UnknownHostError{Name: hostName}
	}
	c, err := Dial(Config{
		Host:       h.Address,
		Port:       p.inv.SSH.Port,
		User:       p.inv.SSH.User,
		PrivateKey: p.inv.SSH.PrivateKey,
	})
	if err != nil {
		return nil, err
	}
	p.clients[hostName] = c
	return c, nil
}

func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.clients {
		_ = c.Close()
	}
	p.clients = map[string]*Client{}
}

type UnknownHostError struct{ Name string }

func (e *UnknownHostError) Error() string { return "unknown host: " + e.Name }
