package cache

import "sync"

// accessNode represents a node in the doubly linked list for LRU
type accessNode struct {
    key  string
    prev *accessNode
    next *accessNode
}

// accessList implements a doubly linked list for LRU tracking
type accessList struct {
    mu      sync.RWMutex
    head    *accessNode
    tail    *accessNode
    nodeMap map[string]*accessNode
}

func newAccessList() *accessList {
    head := &accessNode{}
    tail := &accessNode{}
    head.next = tail
    tail.prev = head

    return &accessList{
        head:    head,
        tail:    tail,
        nodeMap: make(map[string]*accessNode),
    }
}

func (al *accessList) addToFront(key string) {
    al.mu.Lock()
    defer al.mu.Unlock()

    if node, exists := al.nodeMap[key]; exists {
        al.removeNode(node)
    }

    node := &accessNode{key: key}
    al.nodeMap[key] = node
    al.addNodeToFront(node)
}

func (al *accessList) moveToFront(key string) {
    al.mu.Lock()
    defer al.mu.Unlock()

    if node, exists := al.nodeMap[key]; exists {
        al.removeNode(node)
        al.addNodeToFront(node)
    }
}

func (al *accessList) remove(key string) {
    al.mu.Lock()
    defer al.mu.Unlock()

    if node, exists := al.nodeMap[key]; exists {
        al.removeNode(node)
        delete(al.nodeMap, key)
    }
}

func (al *accessList) getLRU() string {
    al.mu.RLock()
    defer al.mu.RUnlock()

    if al.tail.prev == al.head {
        return ""
    }
    return al.tail.prev.key
}

func (al *accessList) addNodeToFront(node *accessNode) {
    node.prev = al.head
    node.next = al.head.next
    al.head.next.prev = node
    al.head.next = node
}

func (al *accessList) removeNode(node *accessNode) {
    node.prev.next = node.next
    node.next.prev = node.prev
	node.prev = nil
	node.next = nil
}