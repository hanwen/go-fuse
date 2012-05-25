package splice
import (
	"sync"
)

var splicePool *pairPool

type pairPool struct {
	sync.Mutex
	unused    []*Pair
	usedCount int
}

func ClearSplicePool() {
	splicePool.clear()
}

func Get() (*Pair, error) {
	return splicePool.get()
}

func Used() int {
	return splicePool.used()
}

func Done(p *Pair) {
	splicePool.done(p)
}

func newSplicePairPool() *pairPool {
	return &pairPool{}
}

func (me *pairPool) clear() {
	me.Lock()
	defer me.Unlock()
	for _, p := range me.unused {
		p.Close()
	}
	me.unused = me.unused[:0]
}

func (me *pairPool) used() int {
	me.Lock()
	defer me.Unlock()
	return me.usedCount
}


func (me *pairPool) get() (p *Pair, err error) {
	me.Lock()
	defer me.Unlock()

	me.usedCount++
	l := len(me.unused)
	if l > 0 {
		p := me.unused[l-1]
		me.unused = me.unused[:l-1]
		return p, nil
	}
	
	return newSplicePair()
}

func (me *pairPool) done(p *Pair) {
	me.Lock()
	defer me.Unlock()

	me.usedCount--
	me.unused = append(me.unused, p)
}

func init() {
	splicePool = newSplicePairPool()
}
