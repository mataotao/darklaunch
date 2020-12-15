darklaunch
======


darklaunch 是一个灰度组件发布的框架，并且可以支持对应的灰度策略和任务，支持同步和异步

#### 安装
	go get git.dustess.com/mk-base/darklaunch


#### 使用

```go

import (
	"github.com/mataotao/darklaunch"
)

func main() {
    d := darklaunch.New()
    t := new(Testtt)
    // 添加一个同步灰度组件 同步执行检测函数，第二个参数是，检测完成执行对应的方法是否同步执行，false表示异步执行
    d.AddFeature(t, false)
    // 添加一个异步灰度组件，第二个参数是，检测完成执行对应的方法是否等待上个检测结果执行完成在执行该任务（同一个检测组件是否排队执行，false表示异步，true排队执行）**** 
    d.AddFeatureAsync(t, false)
    // 灰度检测执行方法，如果检测方法是同步的，则返回对应的检测结果
    // isPass 表示灰度执行结果，bool
    // ptr1，ptr2 表示检测之后执行对应函数的结果，类型都为uintptr,需要开发者自己转成所需类型
    isPass,ptr1,ptr2:=d.Dark("key1", "test")

}
/**
灰度组件结构体必须要有dark字段
tag含义及作用：
key：灰度组件的key，tag内容就是定义的key
dark：灰度检测方法，tag内容就是定义检测方法的名字，自定义
passFunc：灰度检测为true时需要执行的方法，tag内容就是定义的key，自定义，为空则不执行
notThroughFunc：灰度检测为false时需要执行的方法，tag内容就是定义的key，为空则不执行
*/

// Testtt 测试灰度组件
type Testtt struct {
	dark interface{} `key:"key1" dark:"DarkFunc" passFunc:"PassFunc" notThroughFunc:"NotThroughFunc"`
}
/**
灰度检测方法，入参根据自己需要所填写，什么类型都有可以，参数个数也是
返回值必须为bool
*/
// DarkFunc DarkFunc
func (t *Testtt) DarkFunc(i string) bool {
	return true
}
/**
灰度检通过是执行的方法，入参必须和灰度检测方法一样，返回值是可选的，如果填必须是指针类型，并且是两个返回值，
返回值必须为bool
*/
// PassFunc NotThroughFunc
func (t *Testtt) PassFunc(i string) (*bool, *bool) {
	fmt.Println(i, "PassFunc")
	a := true
	return &a, &a
}
/**
灰度检不通过是执行的方法，入参必须和灰度检测方法一样，返回值是可选的，如果填必须是指针类型，并且是两个返回值，
返回值必须为bool
*/
// NotThroughFunc NotThroughFunc
func (t *Testtt) NotThroughFunc(i string) {
	fmt.Println(i, "NotThroughFunc")
}

```

