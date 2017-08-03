package backup

/*
 * This file contains structs and functions related to executing specific
 * queries to gather metadata for the objects handled in predata_general.go.
 */

import (
	"fmt"

	"github.com/greenplum-db/gpbackup/utils"
)

type QueryFunctionDefinition struct {
	Oid               uint32
	SchemaName        string `db:"nspname"`
	FunctionName      string `db:"proname"`
	ReturnsSet        bool   `db:"proretset"`
	FunctionBody      string
	BinaryPath        string
	Arguments         string
	IdentArgs         string
	ResultType        string
	Volatility        string  `db:"provolatile"`
	IsStrict          bool    `db:"proisstrict"`
	IsSecurityDefiner bool    `db:"prosecdef"`
	Config            string  `db:"proconfig"`
	Cost              float32 `db:"procost"`
	NumRows           float32 `db:"prorows"`
	DataAccess        string  `db:"prodataaccess"`
	Language          string
	DependsUpon       []string
}

func GetFunctionDefinitions(connection *utils.DBConn) []QueryFunctionDefinition {
	/*
	 * This query is copied from the dumpFunc() function in pg_dump.c, modified
	 * slightly to also retrieve the function's schema, name, and comment.
	 */
	query := fmt.Sprintf(`
SELECT
	p.oid,
	nspname,
	proname,
	proretset,
	coalesce(prosrc, '') AS functionbody,
	coalesce(probin, '') AS binarypath,
	pg_catalog.pg_get_function_arguments(p.oid) AS arguments,
	pg_catalog.pg_get_function_identity_arguments(p.oid) AS identargs,
	pg_catalog.pg_get_function_result(p.oid) AS resulttype,
	provolatile,
	proisstrict,
	prosecdef,
	(
		coalesce(array_to_string(ARRAY(SELECT 'SET ' || option_name || ' TO ' || option_value
		FROM pg_options_to_table(proconfig)), ' '), '')
	) AS proconfig,
	procost,
	prorows,
	prodataaccess,
	(SELECT lanname FROM pg_catalog.pg_language WHERE oid = prolang) AS language
FROM pg_proc p
LEFT JOIN pg_namespace n
	ON p.pronamespace = n.oid
WHERE %s
AND proisagg = 'f'
ORDER BY nspname, proname, identargs;`, NonUserSchemaFilterClause("n"))

	results := make([]QueryFunctionDefinition, 0)
	err := connection.Select(&results, query)
	utils.CheckError(err)
	return results
}

type QueryAggregateDefinition struct {
	Oid                 uint32
	SchemaName          string `db:"nspname"`
	AggregateName       string `db:"proname"`
	Arguments           string
	IdentArgs           string
	TransitionFunction  uint32 `db:"aggtransfn"`
	PreliminaryFunction uint32 `db:"aggprelimfn"`
	FinalFunction       uint32 `db:"aggfinalfn"`
	SortOperator        uint32 `db:"aggsortop"`
	TransitionDataType  string
	InitialValue        string
	IsOrdered           bool `db:"aggordered"`
}

func GetAggregateDefinitions(connection *utils.DBConn) []QueryAggregateDefinition {
	query := fmt.Sprintf(`
SELECT
	p.oid,
	n.nspname,
	p.proname,
	pg_catalog.pg_get_function_arguments(p.oid) AS arguments,
	pg_catalog.pg_get_function_identity_arguments(p.oid) AS identargs,
	a.aggtransfn::regproc::oid,
	a.aggprelimfn::regproc::oid,
	a.aggfinalfn::regproc::oid,
	a.aggsortop::regproc::oid,
	t.typname as transitiondatatype,
	coalesce(a.agginitval, '') AS initialvalue,
	a.aggordered
FROM pg_aggregate a
LEFT JOIN pg_proc p ON a.aggfnoid = p.oid
LEFT JOIN pg_type t ON a.aggtranstype = t.oid
LEFT JOIN pg_namespace n ON p.pronamespace = n.oid
WHERE %s;`, NonUserSchemaFilterClause("n"))

	results := make([]QueryAggregateDefinition, 0)
	err := connection.Select(&results, query)
	utils.CheckError(err)
	return results
}

type QueryFunction struct {
	Oid            uint32
	FunctionSchema string `db:"nspname"`
	FunctionName   string `db:"proname"`
	Arguments      string
}

type FunctionInfo struct {
	QualifiedName string
	Arguments     string
	IsInternal    bool
}

func GetFunctionOidToInfoMap(connection *utils.DBConn) map[uint32]FunctionInfo {
	query := `
SELECT
	p.oid,
	n.nspname,
	p.proname,
	pg_catalog.pg_get_function_arguments(p.oid) AS arguments
FROM pg_proc p
LEFT JOIN pg_namespace n ON p.pronamespace = n.oid;
`

	results := make([]QueryFunction, 0)
	funcMap := make(map[uint32]FunctionInfo, 0)
	err := connection.Select(&results, query)
	utils.CheckError(err)
	for _, function := range results {
		fqn := MakeFQN(function.FunctionSchema, function.FunctionName)

		isInternal := false
		if function.FunctionSchema == "pg_catalog" {
			isInternal = true
		}
		funcInfo := FunctionInfo{QualifiedName: fqn, Arguments: function.Arguments, IsInternal: isInternal}
		funcMap[function.Oid] = funcInfo
	}
	return funcMap
}

type QueryCastDefinition struct {
	Oid            uint32
	SourceType     string
	TargetType     string
	FunctionSchema string
	FunctionName   string
	FunctionArgs   string
	CastContext    string
}

func GetCastDefinitions(connection *utils.DBConn) []QueryCastDefinition {
	/* This query retrieves all casts where either the source type, the target
	 * type, or the cast function is user-defined.
	 */
	query := fmt.Sprintf(`
SELECT
	c.oid,
	st.typname AS sourcetype,
	tt.typname AS targettype,
	coalesce(n.nspname, '') AS functionschema,
	coalesce(p.proname, '') AS functionname,
	coalesce(pg_get_function_arguments(p.oid), '') AS functionargs,
	c.castcontext
FROM pg_cast c
JOIN pg_type st ON c.castsource = st.oid
JOIN pg_type tt ON c.casttarget = tt.oid
JOIN pg_namespace sn ON st.typnamespace = sn.oid
JOIN pg_namespace tn ON tt.typnamespace = tn.oid
LEFT JOIN pg_proc p ON c.castfunc = p.oid
LEFT JOIN pg_description d ON c.oid = d.objoid
LEFT JOIN pg_namespace n ON p.pronamespace = n.oid
WHERE (%s) OR (%s) OR (%s)
ORDER BY 1, 2;
`, NonUserSchemaFilterClause("sn"), NonUserSchemaFilterClause("tn"), NonUserSchemaFilterClause("n"))

	results := make([]QueryCastDefinition, 0)
	err := connection.Select(&results, query)
	utils.CheckError(err)
	return results
}

/*
 * When we retrieve function dependencies, we don't record a dependency of a
 * function on a base type if the function is part of the definition of the
 * base type, as we print out shell types for all base types at the beginning
 * of the dump and so do not need to consider those dependencies when sorting
 * functions and types.
 */
func ConstructFunctionDependencies(connection *utils.DBConn, functions []QueryFunctionDefinition) []QueryFunctionDefinition {
	query := fmt.Sprintf(`
SELECT
p.oid,
quote_ident(n.nspname) || '.' || quote_ident(t.typname) AS referencedobject
FROM pg_depend d
JOIN pg_type t ON (d.refobjid = t.oid AND t.typtype != 'e' AND t.typtype != 'p')
JOIN pg_proc p ON d.objid = p.oid
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE %s
AND d.refclassid = 'pg_type'::regclass
AND t.typinput != p.oid
AND t.typoutput != p.oid
AND t.typreceive != p.oid
AND t.typsend != p.oid
AND t.typmodin != p.oid
AND t.typmodout != p.oid;`, NonUserSchemaFilterClause("n"))

	results := make([]QueryDependency, 0)
	dependencyMap := make(map[uint32][]string, 0)
	err := connection.Select(&results, query)
	utils.CheckError(err)
	for _, dependency := range results {
		dependencyMap[dependency.Oid] = append(dependencyMap[dependency.Oid], dependency.ReferencedObject)
	}
	for i := 0; i < len(functions); i++ {
		functions[i].DependsUpon = dependencyMap[functions[i].Oid]
	}
	return functions
}