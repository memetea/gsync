package gsync

import (
	"sort"
	"sync"
	"time"
)

type cacheItem struct {
	key   string
	value interface{}

	visitAt time.Time
	expire  time.Duration
	visits  int
}

type byExpireAndVisit []*cacheItem

func (b byExpireAndVisit) Len() int { return len(b) }
func (b byExpireAndVisit) Less(i, j int) bool {
	now := time.Now()
	iExpired := b[i].visitAt.Add(b[i].expire).Before(now)
	jExpired := b[j].visitAt.Add(b[j].expire).Before(now)
	if iExpired && jExpired || !iExpired && !jExpired {
		return b[i].visits < b[j].visits
	} else if iExpired && !jExpired {
		return true
	} else if !iExpired && jExpired {
		return false
	}
	return false
}
func (b byExpireAndVisit) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

type HotCache struct {
	itemsLimit int

	lastExpireCheckTime time.Time
	expireCheckPeriod   time.Duration

	sync.Mutex
	items map[string]*cacheItem
}

func CreateCache(itemsLimit int) *HotCache {
	return &HotCache{
		itemsLimit:        itemsLimit,
		items:             make(map[string]*cacheItem),
		expireCheckPeriod: 10 * time.Minute,
	}
}

func (c *HotCache) Delete(key string) {
	delete(c.items, key)
}

func (c *HotCache) Clear() {
	c.Lock()
	defer c.Unlock()
	c.items = make(map[string]*cacheItem)
}

func (c *HotCache) AddItem(key string, value interface{}, expires time.Duration) {
	c.Lock()
	defer c.Unlock()

	now := time.Now()
	if c.itemsLimit > 0 && len(c.items) > c.itemsLimit ||
		c.lastExpireCheckTime.UnixNano() == 0 ||
		now.After(c.lastExpireCheckTime.Add(c.expireCheckPeriod)) {
		c.lastExpireCheckTime = now
		var items []*cacheItem
		for _, v := range c.items {
			items = append(items, v)
		}
		sort.Sort(byExpireAndVisit(items))

		//clear expired items
		var index int
		for i, v := range items {
			if now.After(v.visitAt.Add(v.expire)) {
				delete(c.items, v.key)
				index = i + 1
			} else {
				break
			}
		}

		if len(c.items) > c.itemsLimit {
			//clear minimal visits items
			clearCount := len(c.items) - c.itemsLimit
			for i := index; i < clearCount; i++ {
				delete(c.items, items[i].key)
			}
		}
	}

	c.items[key] = &cacheItem{
		key:     key,
		value:   value,
		visitAt: time.Now(),
		expire:  expires,
		visits:  0,
	}
}

func (c *HotCache) Get(key string) (interface{}, bool) {
	c.Lock()
	defer c.Unlock()
	if item, ok := c.items[key]; ok {
		item.visitAt = time.Now()
		return item.value, true
	}
	return nil, false
}

func (c *HotCache) GetString(key string) (string, bool) {
	v, ok := c.Get(key)
	if !ok {
		return "", false
	}
	value, ok := v.(string)
	if !ok {
		return "", false
	}
	return value, true
}

func (c *HotCache) GetBytes(key string) ([]byte, bool) {
	v, ok := c.Get(key)
	if !ok {
		return nil, false
	}
	value, ok := v.([]byte)
	if !ok {
		return nil, false
	}
	return value, true
}
