// Directed Acyclic Graph implementation in golang.

package dag

import (
	"sync"
)

type Dag struct {
	vertices sync.Map
}

// New - creates a new Directed Acyclic Graph
func New() *Dag {
	return &Dag{}
}

// AddVertex adds a vertex to the graph
func (d *Dag) AddVertex(vertexID string, value any) (*Vertex, error) {
	// check on an uniq key
	if _, ok := d.vertices.Load(vertexID); ok {
		return nil, &VertexAlreadyExistsError{Id: vertexID}
	}

	node := &Vertex{
		id:       vertexID,
		value:    value,
		parents:  make([]*Vertex, 0),
		children: make([]*Vertex, 0),
	}
	d.vertices.Store(vertexID, node)

	return node, nil
}

func (d *Dag) AddEdge(fromID, toID string) error {
	fromVertex, err := d.GetVertex(fromID)
	if err != nil {
		return err
	}

	toVertex, err := d.GetVertex(toID)
	if err != nil {
		return err
	}

	fromVertex.children = append(fromVertex.children, toVertex)
	toVertex.parents = append(toVertex.parents, fromVertex)

	// update map
	d.vertices.Store(fromID, fromVertex)
	d.vertices.Store(toID, toVertex)

	return nil
}

func (d *Dag) GetVertex(id string) (*Vertex, error) {
	vertex, ok := d.vertices.Load(id)
	if !ok {
		return nil, &VertexNotFoundError{Id: id}
	}

	if v, okAsserion := vertex.(*Vertex); okAsserion {
		return v, nil
	}

	return nil, ErrIncorrectTypeAssertion
}
