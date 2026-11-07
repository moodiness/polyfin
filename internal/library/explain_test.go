package library

// planNode is a node of the plan EXPLAIN (FORMAT JSON) gives.
type planNode struct {
	NodeType  string     `json:"Node Type"`
	Relation  string     `json:"Relation Name"`
	Index     string     `json:"Index Name"`
	IndexCond string     `json:"Index Cond"`
	Removed   int        `json:"Rows Removed by Filter"`
	Plans     []planNode `json:"Plans"`
}
