package renderer

import (
	"fmt"
	"strings"

	"github.com/antonmedv/expr"
	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	"gopkg.in/yaml.v3"
)

type Renderer struct {
	lxd     lxd.InstanceServer
	exprEnv map[string]interface{}
}

func NewRenderer(lxdServer lxd.InstanceServer, extraEnv map[string]interface{}) (*Renderer, error) {
	r := new(Renderer)
	return r, r.Init(lxdServer, extraEnv)
}

func (r *Renderer) Init(lxdServer lxd.InstanceServer, extraEnv map[string]interface{}) error {
	r.lxd = lxdServer
	r.exprEnv = make(map[string]interface{})
	r.exprEnv["getResources"] = func() *api.Resources {
		res, err := r.lxd.GetServerResources()
		if err != nil {
			panic(err)
		}
		return res
	}
	for k, v := range extraEnv {
		r.exprEnv[k] = v
	}
	return nil
}

func (r *Renderer) ExecuteExpression(e string, extraEnv map[string]interface{}) (interface{}, error) {
	trimmedE := strings.Trim(e, "\r\t\n ")
	if !strings.HasPrefix(trimmedE, "${") || !strings.HasSuffix(trimmedE, "}") {
		return e, nil
	}
	trimmedE = trimmedE[2 : len(trimmedE)-1]
	env := make(map[string]interface{})
	for k, v := range r.exprEnv {
		env[k] = v
	}
	for k, v := range extraEnv {
		env[k] = v
	}
	return expr.Eval(trimmedE, env)
}

func (r *Renderer) RenderInstancePut(i api.InstancePut, target *Target) (api.InstancePut, error) {
	extraEnv := target.Data
	for k := range i.Config {
		nv, err := r.ExecuteExpression(i.Config[k], extraEnv)
		if err != nil {
			return i, err
		}
		nvs, ok := nv.(string)
		if !ok {
			return i, fmt.Errorf("value %#v cannot be converted to string", nv)
		}
		i.Config[k] = nvs
	}
	for ki := range i.Devices {
		for k := range i.Devices[ki] {
			nv, err := r.ExecuteExpression(i.Devices[ki][k], extraEnv)
			if err != nil {
				return i, err
			}
			nvs, ok := nv.(string)
			if !ok {
				return i, fmt.Errorf("value %#v cannot be converted to string", nv)
			}
			i.Devices[ki][k] = nvs
		}
	}
	nv, err := r.ExecuteExpression(i.Architecture, extraEnv)
	if err != nil {
		return i, err
	}
	nvs, ok := nv.(string)
	if !ok {
		return i, fmt.Errorf("value %#v cannot be converted to string", nv)
	}
	i.Architecture = nvs
	return i, nil
}

func (r *Renderer) RenderCreate(req api.InstancesPost, target *Target) error {
	var err error
	req.InstancePut, err = r.RenderInstancePut(req.InstancePut, target)
	if err != nil {
		return err
	}
	l := r.lxd
	if target.Target != "" {
		l = l.UseTarget(target.Target)
	}
	op, err := l.CreateInstance(req)
	if err != nil {
		return err
	}
	return op.Wait()
}

func (r *Renderer) RenderStart(name string, req api.InstancePut, target *Target) error {
	var err error
	req, err = r.RenderInstancePut(req, target)
	if err != nil {
		return err
	}
	l := r.lxd
	ins, _, err := l.GetInstance(name)
	if err != nil {
		return err
	}
	if target.Target != "" && ins.Location != target.Target {
		live := false
		if ins.StatusCode == api.Running {
			live = true
		}
		op, err := l.MigrateInstance(name, api.InstancePost{
			Name:         name,
			Migration:    true,
			Live:         live,
			InstanceOnly: true,
		})
		if err != nil {
			return err
		}
		err = op.Wait()
		if err != nil {
			return err
		}
	}
	for k, v := range ins.Config {
		if strings.HasPrefix(k, "volatile.") {
			req.Config[k] = v
		}
	}
	op, err := l.UpdateInstance(name, req, "")
	if err != nil {
		return err
	}
	err = op.Wait()
	if err != nil {
		return err
	}
	if ins.StatusCode == api.Running {
		return nil
	}
	op, err = l.UpdateInstanceState(name, api.InstanceStatePut{
		Action: "start",
	}, "")
	if err != nil {
		return err
	}
	return op.Wait()
}

func YAMLToInstancePost(byt string) (api.InstancesPost, error) {
	var rslt api.InstancesPost
	err := yaml.Unmarshal([]byte(byt), &rslt)
	if err != nil {
		return api.InstancesPost{}, err
	}
	return rslt, nil
}
