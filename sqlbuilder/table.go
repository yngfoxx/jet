// Modeling of tables.  This is where query preparation starts

package sqlbuilder

import (
	"bytes"
	"fmt"
	"github.com/dropbox/godropbox/errors"
)

type TableInterface interface {
	SchemaName() string
	TableName() string
	// Returns the list of columns that are in the current tableName expression.
	Columns() []Column
	// Generates the sql string for the current tableName expression.  Note: the
	// generated string may not be a valid/executable sql statement.
	SerializeSql(out *bytes.Buffer) error
}

// The sql tableName read interface.  NOTE: NATURAL JOINs, and join "USING" clause
// are not supported.
type ReadableTable interface {
	TableInterface

	// Generates a select query on the current tableName.
	SELECT(projections ...Projection) SelectStatement

	// Creates a inner join tableName expression using onCondition.
	INNER_JOIN(table ReadableTable, onCondition BoolExpression) ReadableTable

	// Creates a left join tableName expression using onCondition.
	LeftJoinOn(table ReadableTable, onCondition BoolExpression) ReadableTable

	// Creates a right join tableName expression using onCondition.
	RightJoinOn(table ReadableTable, onCondition BoolExpression) ReadableTable

	FULL_JOIN(table ReadableTable, onCondition BoolExpression) ReadableTable

	CrossJoin(table ReadableTable) ReadableTable
}

// The sql tableName write interface.
type WritableTable interface {
	TableInterface

	INSERT(columns ...Column) InsertStatement
	Update() UpdateStatement
	Delete() DeleteStatement
}

// Defines a physical tableName in the database that is both readable and writable.
// This function will panic if name is not valid
func NewTable(schemaName, name string, columns ...Column) *Table {
	if !validIdentifierName(name) {
		panic("Invalid tableName name")
	}

	t := &Table{
		schemaName:   schemaName,
		name:         name,
		columns:      columns,
		columnLookup: make(map[string]Column),
	}
	for _, c := range columns {
		err := c.setTableName(name)
		if err != nil {
			panic(err)
		}
		t.columnLookup[c.TableName()] = c
	}

	if len(columns) == 0 {
		panic(fmt.Sprintf("Table %s has no columns", name))
	}

	return t
}

type Table struct {
	schemaName   string
	name         string
	alias        string
	columns      []Column
	columnLookup map[string]Column
	// If not empty, the name of the index to force
	forcedIndex string
}

// Returns the specified column, or errors if it doesn't exist in the tableName
func (t *Table) getColumn(name string) (Column, error) {
	if c, ok := t.columnLookup[name]; ok {
		return c, nil
	}
	return nil, errors.Newf("No such column '%s' in tableName '%s'", name, t.name)
}

func (t *Table) Column(name string) Column {
	return &baseColumn{
		name:      name,
		nullable:  NotNullable,
		tableName: t.name,
	}
}

// Returns all expresssion for a tableName as a slice of projections
func (t *Table) Projections() []Expression {
	result := make([]Expression, 0)

	for _, col := range t.columns {
		col.Asc()
		result = append(result, col)
	}

	return result
}

func (t *Table) SetAlias(alias string) {
	t.alias = alias

	for _, c := range t.columns {
		err := c.setTableName(alias)
		if err != nil {
			panic(err)
		}
	}
}

// Returns the tableName's name in the database
func (t *Table) SchemaName() string {
	return t.schemaName
}

// Returns the tableName's name in the database
func (t *Table) TableName() string {
	return t.name
}

func (t *Table) SchemaTableName() string {
	return t.schemaName
}

// Returns a list of the tableName's columns
func (t *Table) Columns() []Column {
	return t.columns
}

// Returns a copy of this tableName, but with the specified index forced.
func (t *Table) ForceIndex(index string) *Table {
	newTable := *t
	newTable.forcedIndex = index
	return &newTable
}

// Generates the sql string for the current tableName expression.  Note: the
// generated string may not be a valid/executable sql statement.
func (t *Table) SerializeSql(out *bytes.Buffer) error {
	if !validIdentifierName(t.schemaName) {
		return errors.New("Invalid database name specified")
	}

	_, _ = out.WriteString(t.schemaName)
	_, _ = out.WriteString(".")
	_, _ = out.WriteString(t.TableName())

	if len(t.alias) > 0 {
		out.WriteString(" AS ")
		out.WriteString(t.alias)
	}

	if t.forcedIndex != "" {
		if !validIdentifierName(t.forcedIndex) {
			return errors.Newf("'%s' is not a valid identifier for an index", t.forcedIndex)
		}
		_, _ = out.WriteString(" FORCE INDEX (")
		_, _ = out.WriteString(t.forcedIndex)
		_, _ = out.WriteString(")")
	}

	return nil
}

// Generates a select query on the current tableName.
func (t *Table) SELECT(projections ...Projection) SelectStatement {
	return newSelectStatement(t, projections)
}

// Creates a inner join tableName expression using onCondition.
func (t *Table) INNER_JOIN(
	table ReadableTable,
	onCondition BoolExpression) ReadableTable {

	return InnerJoinOn(t, table, onCondition)
}

//func (t *Table) InnerJoinUsing(
//	table ReadableTable,
//	col1 Column,
//	col2 Column) ReadableTable {
//
//	return INNER_JOIN(t, table, col1.Eq(col2))
//}

// Creates a left join tableName expression using onCondition.
func (t *Table) LeftJoinOn(
	table ReadableTable,
	onCondition BoolExpression) ReadableTable {

	return LeftJoinOn(t, table, onCondition)
}

// Creates a right join tableName expression using onCondition.
func (t *Table) RightJoinOn(
	table ReadableTable,
	onCondition BoolExpression) ReadableTable {

	return RightJoinOn(t, table, onCondition)
}

func (t *Table) FULL_JOIN(table ReadableTable, onCondition BoolExpression) ReadableTable {
	return FullJoin(t, table, onCondition)
}

func (t *Table) CrossJoin(table ReadableTable) ReadableTable {
	return CrossJoin(t, table)
}

func (t *Table) INSERT(columns ...Column) InsertStatement {
	return newInsertStatement(t, columns...)
}

func (t *Table) Update() UpdateStatement {
	return newUpdateStatement(t)
}

func (t *Table) Delete() DeleteStatement {
	return newDeleteStatement(t)
}

type joinType int

const (
	INNER_JOIN joinType = iota
	LEFT_JOIN
	RIGHT_JOIN
	FULL_JOIN
	CROSS_JOIN
)

// Join expressions are pseudo readable tables.
type joinTable struct {
	lhs         ReadableTable
	rhs         ReadableTable
	join_type   joinType
	onCondition BoolExpression
}

func newJoinTable(
	lhs ReadableTable,
	rhs ReadableTable,
	join_type joinType,
	onCondition BoolExpression) ReadableTable {

	return &joinTable{
		lhs:         lhs,
		rhs:         rhs,
		join_type:   join_type,
		onCondition: onCondition,
	}
}

func InnerJoinOn(
	lhs ReadableTable,
	rhs ReadableTable,
	onCondition BoolExpression) ReadableTable {

	return newJoinTable(lhs, rhs, INNER_JOIN, onCondition)
}

func LeftJoinOn(
	lhs ReadableTable,
	rhs ReadableTable,
	onCondition BoolExpression) ReadableTable {

	return newJoinTable(lhs, rhs, LEFT_JOIN, onCondition)
}

func RightJoinOn(
	lhs ReadableTable,
	rhs ReadableTable,
	onCondition BoolExpression) ReadableTable {

	return newJoinTable(lhs, rhs, RIGHT_JOIN, onCondition)
}

func FullJoin(
	lhs ReadableTable,
	rhs ReadableTable,
	onCondition BoolExpression) ReadableTable {

	return newJoinTable(lhs, rhs, FULL_JOIN, onCondition)
}

func CrossJoin(
	lhs ReadableTable,
	rhs ReadableTable) ReadableTable {

	return newJoinTable(lhs, rhs, CROSS_JOIN, nil)
}

// Returns the tableName's name in the database
func (t *joinTable) SchemaName() string {
	return ""
}

func (t *joinTable) TableName() string {
	return ""
}

func (t *joinTable) Columns() []Column {
	columns := make([]Column, 0)
	columns = append(columns, t.lhs.Columns()...)
	columns = append(columns, t.rhs.Columns()...)

	return columns
}

func (t *joinTable) Column(name string) Column {
	panic("Not implemented")
}

func (t *joinTable) SerializeSql(out *bytes.Buffer) (err error) {

	if t.lhs == nil {
		return errors.Newf("nil lhs.  Generated sql: %s", out.String())
	}
	if t.rhs == nil {
		return errors.Newf("nil rhs.  Generated sql: %s", out.String())
	}
	if t.onCondition == nil && t.join_type != CROSS_JOIN {
		return errors.Newf("nil onCondition.  Generated sql: %s", out.String())
	}

	if err = t.lhs.SerializeSql(out); err != nil {
		return
	}

	switch t.join_type {
	case INNER_JOIN:
		_, _ = out.WriteString(" JOIN ")
	case LEFT_JOIN:
		_, _ = out.WriteString(" LEFT JOIN ")
	case RIGHT_JOIN:
		_, _ = out.WriteString(" RIGHT JOIN ")
	case FULL_JOIN:
		out.WriteString(" FULL JOIN ")
	case CROSS_JOIN:
		out.WriteString(" CROSS JOIN ")
	}

	if err = t.rhs.SerializeSql(out); err != nil {
		return
	}

	if t.onCondition != nil {
		_, _ = out.WriteString(" ON ")
		if err = t.onCondition.SerializeSql(out); err != nil {
			return
		}
	}

	return nil
}

func (t *joinTable) SELECT(projections ...Projection) SelectStatement {
	return newSelectStatement(t, projections)
}

func (t *joinTable) INNER_JOIN(
	table ReadableTable,
	onCondition BoolExpression) ReadableTable {

	return InnerJoinOn(t, table, onCondition)
}

func (t *joinTable) LeftJoinOn(
	table ReadableTable,
	onCondition BoolExpression) ReadableTable {

	return LeftJoinOn(t, table, onCondition)
}

func (t *joinTable) FULL_JOIN(table ReadableTable, onCondition BoolExpression) ReadableTable {
	return FullJoin(t, table, onCondition)
}

func (t *joinTable) CrossJoin(table ReadableTable) ReadableTable {
	return CrossJoin(t, table)
}

func (t *joinTable) RightJoinOn(
	table ReadableTable,
	onCondition BoolExpression) ReadableTable {

	return RightJoinOn(t, table, onCondition)
}
