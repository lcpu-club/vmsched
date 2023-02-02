package renderer

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

type RawTargets struct {
	Targets map[string]interface{} `yaml:"targets" json:"targets"`
}

type Targets struct {
	Target string                   `yaml:"target" json:"target"`
	Count  int                      `yaml:"count" json:"count"`
	Each   map[string][]interface{} `yaml:"each" json:"each"`
	Common map[string]interface{}   `yaml:"common" json:"common"`
}

type Target struct {
	Target string                 `yaml:"target" json:"target"`
	Data   map[string]interface{} `yaml:"data" json:"data"`
}

func ParseTargets(r *Renderer, t []byte) ([]*Target, error) {
	rt := &RawTargets{
		Targets: map[string]interface{}{},
	}
	err := yaml.Unmarshal(t, rt)
	if err != nil {
		return nil, err
	}
	targets := []*Targets{}
	for nodeRaw, contentRaw := range rt.Targets {
		content, ok := contentRaw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%#v is not map[string]interface{}", content)
		}
		nodes := []string{}
		if strings.HasPrefix(nodeRaw, "@") {
			gr, _, err := r.lxd.GetClusterGroup(nodeRaw[1:])
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, gr.Members...)
		} else {
			nodes = append(nodes, nodeRaw)
		}
		for _, node := range nodes {
			r.lxd = r.lxd.UseTarget(node)
			extraEnv := map[string]interface{}{
				"target": node,
			}
			count, counted := 0, true
			countRaw, ok := content["count"]
			if ok {
				if cStr, ok := countRaw.(string); ok {
					cRslt, err := r.ExecuteExpression(cStr, extraEnv)
					if err != nil {
						return nil, err
					}
					if count, ok = cRslt.(int); !ok {
						return nil, fmt.Errorf("invalid count value %#v", count)
					}
				} else if count, ok = countRaw.(int); !ok {
					return nil, fmt.Errorf("invalid count value %#v", countRaw)
				}
			} else {
				counted = false
			}
			each := make(map[string][]interface{})
			if eachRaw, ok := content["each"]; ok {
				eachMixed, ok := eachRaw.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("invalid each value %#v", eachRaw)
				}
				for k, vm := range eachMixed {
					var val []interface{}
					var ok bool
					if val, ok = vm.([]interface{}); !ok {
						if e, ok := vm.(string); ok {
							r, err := r.ExecuteExpression(e, extraEnv)
							if err != nil {
								return nil, err
							}
							if reflect.TypeOf(r).Kind() != reflect.Slice {
								return nil, fmt.Errorf("invalid each expression return value %#v", r)
							}
							rv := reflect.ValueOf(r)
							for i := 0; i < rv.Len(); i++ {
								val = append(val, rv.Index(i).Interface())
							}
						} else {
							return nil, fmt.Errorf("invalid each value %#v", eachMixed)
						}
					} else {
						for vk := range val {
							if valStr, ok := val[vk].(string); ok {
								val[vk], err = r.ExecuteExpression(valStr, extraEnv)
								if err != nil {
									return nil, err
								}
							}
						}
					}
					each[k] = val
					if !counted {
						count = len(val)
					} else {
						if count != len(val) {
							return nil, fmt.Errorf("value %#v's length is not %v", val, count)
						}
					}
				}
			}
			common := make(map[string]interface{})
			if commonRaw, ok := content["common"]; ok {
				common, ok = commonRaw.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("invalid common value %#v", commonRaw)
				}
				for k := range common {
					if vStr, ok := common[k].(string); ok {
						common[k], err = r.ExecuteExpression(vStr, extraEnv)
						if err != nil {
							return nil, err
						}
					}
				}
			}
			targets = append(targets, &Targets{
				Target: node,
				Count:  count,
				Each:   each,
				Common: common,
			})
		}
	}
	rslt := []*Target{}
	for _, t := range targets {
		for i := 0; i < t.Count; i++ {
			item := &Target{
				Target: t.Target,
				Data:   make(map[string]interface{}),
			}
			for k, v := range t.Common {
				item.Data[k] = v
			}
			for k, vs := range t.Each {
				item.Data[k] = vs[i]
			}
			rslt = append(rslt, item)
		}
	}
	return rslt, nil
}
