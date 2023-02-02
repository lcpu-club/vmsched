package models

import "xorm.io/xorm"

func Sync(orm *xorm.Engine) error {
	return orm.Sync(
		InstanceType{},
		InstanceTarget{},
		User{},
		Token{},
		Task{},
		Queue{},
	)
}
