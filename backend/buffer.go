package backend

type (
	Buffer struct {
		data      string
		callbacks []BufferChangedCallback
	}
	BufferChangedCallback func(position, delta int)
)

func (b *Buffer) Size() int {
	return len(b.data)
}

func (buf *Buffer) Substr(r Region) string {
	l := len(buf.data)
	a, b := clamp(0, l, r.Begin()), clamp(0, l, r.End())
	return string(buf.data[a:b])
}

func (buf *Buffer) notify(position, delta int) {
	for i := range buf.callbacks {
		buf.callbacks[i](position, delta)
	}
}

func (buf *Buffer) Insert(point int, value string) {
	buf.data = buf.data[0:point] + value + buf.data[point:len(buf.data)]
	buf.notify(point, len(value))
}

func (buf *Buffer) Erase(point, length int) {
	buf.data = buf.data[0:point] + buf.data[point+length:len(buf.data)]
	buf.notify(point, -length)
}
