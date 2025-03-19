package queue

// EventType 表示队列事件的类型
type EventType int

const (
	// EventEnqueue 元素入队事件
	EventEnqueue EventType = iota

	// EventDequeue 元素出队事件
	EventDequeue

	// EventFull 队列满事件
	EventFull

	// EventEmpty 队列空事件
	EventEmpty

	// EventClose 队列关闭事件
	EventClose

	// EventError 操作错误事件
	EventError
)

// Event 表示队列中发生的事件
type Event struct {
	// 事件类型
	Type EventType

	// 事件发生时队列中的元素数量
	Size int

	// 与事件相关联的元素（如果有）
	Item interface{}

	// 与事件相关联的错误（如果有）
	Err error
}

// EventListener 是接收队列事件的函数接口
type EventListener func(Event)

// EventEmitter 提供事件通知功能
type EventEmitter struct {
	listeners []EventListener
}

// NewEventEmitter 创建一个新的事件发射器
func NewEventEmitter(listeners []EventListener) *EventEmitter {
	if listeners == nil {
		listeners = []EventListener{}
	}
	return &EventEmitter{
		listeners: listeners,
	}
}

// AddListener 添加一个事件监听器
func (e *EventEmitter) AddListener(listener EventListener) {
	e.listeners = append(e.listeners, listener)
}

// Emit 发送事件给所有监听器
func (e *EventEmitter) Emit(evt Event) {
	for _, listener := range e.listeners {
		listener(evt)
	}
}
