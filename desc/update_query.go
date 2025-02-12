package desc

import (
	"fmt"
	"strconv"
	"strings"
)

// BuildUpdateQuery builds and returns an SQL query for updating a row in the table,
// using the given struct value and the primary key.
func BuildUpdateQuery(value any, columnsToUpdate []string, primaryKey *Column) (string, []any, error) {
	args, err := extractUpdateArguments(value, columnsToUpdate, primaryKey)
	if err != nil {
		return "", nil, err
	}

	shouldUpdateID := false
	for _, col := range columnsToUpdate {
		if col == primaryKey.Name {
			shouldUpdateID = true
			break
		}
	}

	if len(args) == 1 { // the last one is the id.
		return "", nil, fmt.Errorf("no arguments found for update, maybe missing struct field tag of \"%s\"", DefaultTag)
	}

	// build the SQL query using the table definition and its primary key.
	query := buildUpdateQuery(primaryKey.Table, args, primaryKey.Name, shouldUpdateID)
	return query, args.Values(), nil
}

// extractUpdateArguments extracts the arguments from the given struct value and returns them.
func extractUpdateArguments(value any, columnsToUpdate []string, primaryKey *Column) (Arguments, error) {
	structValue := IndirectValue(value)

	id, err := extractPrimaryKeyValue(primaryKey, structValue)
	if err != nil {
		return nil, err
	}

	args, err := extractArguments(primaryKey.Table, structValue)
	if err != nil {
		return nil, err // return the error if finding arguments fails
	}
	
	if len(columnsToUpdate) == 0 {
		for _, arg := range args {
			c := arg.Column
			columnsToUpdate = append(columnsToUpdate, c.Name)
		}
	}
	
	if len(columnsToUpdate) > 0 { // if specific columns to update, then override the default behavior.
		args = filterArguments(args, func(arg Argument) bool {
			for _, onlyColumnName := range columnsToUpdate {
				if arg.Column.Name == onlyColumnName {
					return true
				}
			}

			return false
		})
	} else { // otherwise full update, even zero values (e.g. integer 0) all except ID and any created_at, updated_at.
		args = filterArgumentsForFullUpdate(args)
	}

	if len(args) == 0 {
		// nothing to update, raise an error
		return nil, fmt.Errorf(`no arguments found for update, maybe missing struct field tag of "%s"`, DefaultTag)
	}

	// add the primary key value as the last argument
	args = append(args, Argument{
		Column: primaryKey,
		Value:  id,
	})

	return args, nil
}

func buildUpdateQuery(td *Table, args Arguments, primaryKeyName string, shouldUpdateID bool) string {
	var b strings.Builder

	b.WriteString(`UPDATE "` + td.Name + `" SET `)

	var paramIndex int

	for i, a := range args {
		c := a.Column

		if !shouldUpdateID && c.Name == primaryKeyName {
			// Do not update ID if not specifically asked to.
			// Fixes #1.
			continue
		}

		if i > 0 {
			b.WriteByte(',')
		}

		paramIndex++ // starts from 1.
		paramIndexStr := strconv.Itoa(paramIndex)
		paramName := "$" + paramIndexStr

		if c.Password {
			if td.PasswordHandler.canEncrypt() {
				// handled at args state.
			} else {
				paramName = buildInsertPassword(paramName)
			}
		}

		b.WriteString(fmt.Sprintf("%s = %s", c.Name, paramName))
	}

	primaryKeyWhereIndex := paramIndex + 1
	if shouldUpdateID { // if updating ID, then the last argument is the ID.
		primaryKeyWhereIndex = paramIndex
	}
	b.WriteString(` WHERE "` + primaryKeyName + `" = $` + strconv.Itoa(primaryKeyWhereIndex))

	b.WriteByte(';')

	return b.String()
}
