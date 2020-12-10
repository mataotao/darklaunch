package darklaunch

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// DarkFeature 特性
type DarkFeature interface {
	AddFeature(handler interface{}, asyncExec bool)
	AddFeatureAsync(handler interface{}, transactional bool)
	RemoveFeature(handler interface{})
}

// DarkCheck 检查
type DarkCheck interface {
	Dark(key string, args ...interface{}) (bool, uintptr, uintptr)
}

// DarkController 检查
type DarkController interface {
	SetMaxTaskNum(num int64)
	Preview() map[string]string
	HasDark(key string) bool
}

// Dark 灰度发布
type Dark interface {
	DarkController
	DarkFeature
	DarkCheck
}

// DarkLaunch 灰度发布
type DarkLaunch struct {
	handlers *sync.Map
	lock     *sync.Mutex
	maxTask  chan struct{}
}

type darkHandler struct {
	handlerType   reflect.Type
	handler       interface{}
	dark          reflect.Value // 检测方法
	pass          reflect.Value // 通过时执行
	notThrough    reflect.Value // 未通过时执行
	asyncDark     bool          // 异步检测
	asyncExec     bool          // 异步执行检测之后的方法
	transactional bool
	lock          *sync.Mutex
}

// New new
func New() Dark {
	dark := &DarkLaunch{
		new(sync.Map),
		new(sync.Mutex),
		make(chan struct{}, 200),
	}
	return Dark(dark)
}

// doSubscribe 处理订阅逻辑
func (darkLaunch *DarkLaunch) doSubscribe(key string, handler *darkHandler) error {
	darkLaunch.handlers.Store(key, handler)
	return nil
}

func (darkLaunch *DarkLaunch) checkHandler(darkLaunchHandler interface{}) ([]string, reflect.Type, string, string, string) {
	var t reflect.Type
	var darkFunc string
	var passFunc string
	var notThroughFunc string
	var ok bool

	t = reflect.TypeOf(darkLaunchHandler)
	if t.Elem().Kind() != reflect.Struct {
		panic(fmt.Sprintf("%s is not of type reflect.Struct", t.Kind()))
	}
	fnField, _ := t.Elem().FieldByName("dark")
	if fnField.Tag == "" {
		panic(fmt.Sprintf("%v has no field or no fn field", fnField))
	}
	darkFunc, ok = fnField.Tag.Lookup("dark")
	if !ok || darkFunc == "" {
		panic("dark tag doesn't exist or empty")
	}
	passFunc, _ = fnField.Tag.Lookup("passFunc")
	notThroughFunc, _ = fnField.Tag.Lookup("notThroughFunc")

	key, ok := fnField.Tag.Lookup("key")
	if !ok || key == "" {
		panic("topic tag doesn't exist or empty")
	}
	keys := strings.Split(key, ",")
	return keys, t, darkFunc, passFunc, notThroughFunc
}

func (darkLaunch *DarkLaunch) register(handler interface{}, asyncDark, asyncExec, transactional bool) {
	keys, t, darkFunc, passFunc, notThroughFunc := darkLaunch.checkHandler(handler)
	// 检测函数
	dark, ok := t.MethodByName(darkFunc)
	if !ok {
		return
	}
	// 通过时执行函数
	pass, _ := t.MethodByName(passFunc)

	// 未通过时执行函数
	notThrough, _ := t.MethodByName(notThroughFunc)
	for i := range keys[:] {
		_ = darkLaunch.doSubscribe(keys[i], &darkHandler{
			t, handler, dark.Func, pass.Func, notThrough.Func, asyncDark, asyncExec, transactional, new(sync.Mutex),
		})
	}
}

// AddFeature 添加灰度特性
func (darkLaunch *DarkLaunch) AddFeature(handler interface{}, asyncExec bool) {
	darkLaunch.register(handler, false, asyncExec, false)
}

// AddFeatureAsync  添加灰度特性-异步
func (darkLaunch *DarkLaunch) AddFeatureAsync(handler interface{}, transactional bool) {
	darkLaunch.register(handler, true, false, transactional)

}

// HasDark 查看检测灰度特性的函数
func (darkLaunch *DarkLaunch) HasDark(key string) bool {
	_, ok := darkLaunch.handlers.Load(key)
	return ok
}

// RemoveFeature 删除特性
func (darkLaunch *DarkLaunch) RemoveFeature(handler interface{}) {
	key, _, _, _, _ := darkLaunch.checkHandler(handler)
	for i := range key[:] {
		darkLaunch.removeHandler(key[i])
	}

}

func (darkLaunch *DarkLaunch) removeHandler(topic string) {
	darkLaunch.handlers.Delete(topic)
}

// Dark 检测
func (darkLaunch *DarkLaunch) Dark(key string, args ...interface{}) (bool, uintptr, uintptr) {
	handler, ok := darkLaunch.handlers.Load(key)
	if !ok {
		return false, 0, 0
	}
	darkHandler, ok := handler.(*darkHandler)
	if !ok {
		return false, 0, 0
	}
	if !darkHandler.asyncDark {
		ok := darkLaunch.doDark(darkHandler, args...)
		if !darkHandler.asyncExec {
			ptr1, ptr2 := darkLaunch.doExec(darkHandler, ok, args...)
			return ok, ptr1, ptr2
		}
		darkLaunch.maxTask <- struct{}{}
		go darkLaunch.doExecAsync(darkHandler, ok, args...)
		return ok, 0, 0
	}

	if darkHandler.transactional {
		darkHandler.lock.Lock()
	}
	darkLaunch.maxTask <- struct{}{}
	go darkLaunch.doDarkAsync(darkHandler, args...)
	return true, 0, 0
}

func (darkLaunch *DarkLaunch) doDark(handler *darkHandler, args ...interface{}) bool {
	defer func() {
		if err := recover(); err != nil {
			// log.Errorf("darkLaunch doDark catch err:%s", err)
		}
	}()
	passedArguments := darkLaunch.setUpDark(handler, handler.dark.Type(), args...)
	result := handler.dark.Call(passedArguments)
	if len(result) != 1 {
		return false
	}
	if result[0].Kind() != reflect.Bool {
		return false
	}
	return result[0].Bool()
}

func (darkLaunch *DarkLaunch) doPass(handler *darkHandler, args ...interface{}) (uintptr, uintptr) {
	if !handler.pass.IsValid() {
		return 0, 0
	}
	passedArguments := darkLaunch.setUpDark(handler, handler.pass.Type(), args...)
	result := handler.pass.Call(passedArguments)
	if len(result) == 0 || len(result) > 2 {
		return 0, 0
	}
	return result[0].Pointer(), result[1].Pointer()
}
func (darkLaunch *DarkLaunch) doNotThroughFunc(handler *darkHandler, args ...interface{}) (uintptr, uintptr) {
	if !handler.notThrough.IsValid() {
		return 0, 0
	}
	passedArguments := darkLaunch.setUpDark(handler, handler.notThrough.Type(), args...)
	result := handler.notThrough.Call(passedArguments)
	if len(result) == 0 || len(result) > 2 {
		return 0, 0
	}
	return result[0].Pointer(), result[1].Pointer()
}
func (darkLaunch *DarkLaunch) doExec(handler *darkHandler, isPass bool, args ...interface{}) (uintptr, uintptr) {
	defer func() {
		if err := recover(); err != nil {
			// log.Errorf("darkLaunch doExec catch err:%s", err)
		}
	}()
	if isPass {
		return darkLaunch.doPass(handler, args...)
	}
	return darkLaunch.doNotThroughFunc(handler, args...)
}

func (darkLaunch *DarkLaunch) doExecAsync(handler *darkHandler, isPass bool, args ...interface{}) (uintptr, uintptr) {
	defer func() {
		<-darkLaunch.maxTask
		if err := recover(); err != nil {
			// log.Errorf("darkLaunch doExecAsync catch err:%s", err)
		}
	}()
	if isPass {
		return darkLaunch.doPass(handler, args...)
	}
	return darkLaunch.doNotThroughFunc(handler, args...)
}

func (darkLaunch *DarkLaunch) doDarkAsync(handler *darkHandler, args ...interface{}) {
	defer func() {
		<-darkLaunch.maxTask
		if err := recover(); err != nil {
			// log.Errorf("darkLaunch doDarkAsync catch err:%s", err)
		}
	}()
	if handler.transactional {
		defer handler.lock.Unlock()
	}
	darkLaunch.doExec(handler, darkLaunch.doDark(handler, args...), args...)
}

func (darkLaunch *DarkLaunch) setUpDark(callback *darkHandler, funcType reflect.Type, args ...interface{}) []reflect.Value {
	passedArguments := make([]reflect.Value, 0, len(args)+1)
	passedArguments = append(passedArguments, reflect.ValueOf(callback.handler))
	for i := range args[:] {
		if args[i] == nil {
			passedArguments = append(passedArguments, reflect.New(funcType.In(i)).Elem())
		} else {
			passedArguments = append(passedArguments, reflect.ValueOf(args[i]))
		}
	}

	return passedArguments
}

// SetMaxTaskNum 设置最大执行任务数
func (darkLaunch *DarkLaunch) SetMaxTaskNum(num int64) {
	darkLaunch.maxTask = make(chan struct{}, num)
}

// Preview 预览任务
func (darkLaunch *DarkLaunch) Preview() map[string]string {
	var s strings.Builder
	res := make(map[string]string)
	darkLaunch.handlers.Range(func(key, value interface{}) bool {
		handler := value.(*darkHandler)
		k := key.(string)
		s.WriteString(fmt.Sprintf("\n-------------------------\n%s:\n", k))
		res[k] = handler.dark.String()
		s.WriteString(fmt.Sprintf("%s\n", handler.dark.String()))
		return true
	})
	fmt.Println(s.String())
	return res
}
