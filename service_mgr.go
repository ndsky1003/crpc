package crpc

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"

	"github.com/sirupsen/logrus"
)

type service_mgr struct {
	sync.Mutex
	services   []*service
	weight_sum int
}

func (this *service_mgr) RandOne() *service {
	this.Lock()
	defer this.Unlock()
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

func (this *service_mgr) addService(s *service) error {
	if s.name == "" {
		return errors.New("service name is empty")
	}
	if s.weight == 0 {
		return errors.New("service weight is zero")
	}
	this.Lock()
	defer this.Unlock()
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
			return fmt.Errorf("exists service:%v weight:%d < weight:%d", s.name, this.weight_sum, s.weight)
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
	logrus.Info("add service:", s.name)
	return nil
}

func (this *service_mgr) removeService(s *service, isClose bool) (int, error) {
	if s == nil {
		return this.weight_sum, nil
	}
	this.Lock()
	defer this.Unlock()
	var index int
	for i, ss := range this.services {
		if ss.fingerprint == s.fingerprint {
			index = i
		}
	}
	this.services = append(this.services[:index], this.services[index+1:]...)
	if isClose {
		if err := s.close(false); err != nil {
			logrus.Error(err)
		}
	}
	this.weight_sum -= s.weight
	return this.weight_sum, nil
}
