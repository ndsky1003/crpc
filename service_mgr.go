package crpc

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"

	"github.com/sirupsen/logrus"
)

type service_mgr struct {
	rwl        sync.RWMutex
	services   []*service
	weight_sum int
}

func new_service_mgr() *service_mgr {
	s := &service_mgr{}
	return s
}

func (this *service_mgr) RandOne() *service {
	this.rwl.RLock()
	defer this.rwl.RUnlock()
	if len(this.services) > 0 {
		if this.weight_sum > 0 {
			r := rand.IntN(this.weight_sum)
			for _, s := range this.services {
				r -= s.weight
				if r < 0 {
					return s
				}
			}
		} else {
			return this.services[0]
		}
	}
	return nil
}

func (this *service_mgr) GetService(sid uint32) *service {
	this.rwl.RLock()
	defer this.rwl.RUnlock()
	for _, s := range this.services {
		if s.id == sid {
			return s
		}
	}
	return nil
}

func (this *service_mgr) addService(s *service) error {
	if s.name == "" {
		return errors.New("service name is empty")
	}
	if s.weight == 0 {
		return errors.New("service weight is zero")
	}
	this.rwl.Lock()
	defer this.rwl.Unlock()
	switch {
	case this.weight_sum == 0:
		this.weight_sum = s.weight
		this.services = []*service{s}
	case this.weight_sum < 0:
		if s.weight < this.weight_sum {
			for _, ss := range this.services {
				if err := ss.close(false); err != nil {
					logrus.Error(err)
				}
			}
			this.weight_sum = s.weight
			this.services = []*service{s}
		} else {
			err := fmt.Errorf("exists service:%v weight_sum:%d < weight:%d", s.name, this.weight_sum, s.weight)
			return err
		}
	case this.weight_sum > 0:
		if s.weight < 0 {
			for _, ss := range this.services {
				if err := ss.close(false); err != nil {
					logrus.Error(err)
				}
			}
			this.weight_sum = s.weight
			this.services = []*service{s}
		} else {
			this.weight_sum += s.weight
			this.services = append(this.services, s)
		}
	}
	logrus.Infof("add service:%v weight:%d id:%d", s.name, s.weight, s.id)
	return nil
}

func (this *service_mgr) removeService(s *service, isClose bool) (int, error) {
	if s == nil {
		return this.weight_sum, nil
	}
	this.rwl.Lock()
	defer this.rwl.Unlock()
	index := -1
	for i, ss := range this.services {
		if ss.id == s.id {
			index = i
		}
	}
	if index != -1 {
		this.services = append(this.services[:index], this.services[index+1:]...)
		if isClose {
			if err := s.close(false); err != nil {
				logrus.Error(err)
			}
		}
		switch {
		case this.weight_sum > 0 && s.weight > 0:
			this.weight_sum -= s.weight
		case this.weight_sum < 0 && s.weight < 0 && this.weight_sum == s.weight:
			this.weight_sum = 0
		}
	}
	return this.weight_sum, nil
}
