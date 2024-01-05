package controller

import (
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/raft"
)

type (
	KeyValueController interface {
		Delete(key string) error
		Set(data []byte, timeout time.Duration) (*ApplyResponse, error)
		Get(key string) ([]byte, error)
		Iterate()
	}
)

func (c *controller) Delete(key string) error {
	if err := c.db.Delete(key); err != nil {
		return err
	}

	return nil
}

func (c *controller) Get(key string) ([]byte, error) {
	val, err := c.db.Get(key)
	if err != nil {
		fmt.Println("error: ", err)
		return nil, err
	}

	return val, nil
}

func (c *controller) Iterate() {
	c.db.Iterate()
}

func (c *controller) Set(data []byte, timeout time.Duration) (*ApplyResponse, error) {
	if c.raft.State() != raft.Leader {
		fmt.Println("no leader")
		// todo -> redirect request to leader, or just return an error that the client
		// can then call the leader
		fmt.Println("leader address: ", c.raft.Leader())
		return nil, errors.New("call leader to set data")
	}

	future := c.raft.Apply(data, timeout)
	if err := future.Error(); err != nil {
		fmt.Println(err)
		return nil, errors.New("failed to apply command")
	}

	res, ok := future.Response().(*ApplyResponse)
	if !ok {
		return nil, errors.New("failed to apply command")
	}

	return res, nil
}
