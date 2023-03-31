package ledger_test

import (
	"github.com/blinklabs-io/gouroboros/ledger"
	"testing"
)

type getEraByIdTestDefinition struct {
	Id        uint8
	Name      string
	ExpectNil bool
}

var getEraByIdTests = []getEraByIdTestDefinition{
	{
		Id:   0,
		Name: "Byron",
	},
	{
		Id:   1,
		Name: "Shelley",
	},
	{
		Id:   2,
		Name: "Allegra",
	},
	{
		Id:   3,
		Name: "Mary",
	},
	{
		Id:   4,
		Name: "Alonzo",
	},
	{
		Id:   5,
		Name: "Babbage",
	},
	{
		Id:        99,
		ExpectNil: true,
	},
}

func TestGetEraById(t *testing.T) {
	for _, test := range getEraByIdTests {
		era := ledger.GetEraById(test.Id)
		if era == nil {
			if !test.ExpectNil {
				t.Fatalf("got unexpected nil, wanted %s", test.Name)
			}
		} else {
			if era.Name != test.Name {
				t.Fatalf("did not get expected era name for ID %d, got: %s, wanted: %s", test.Id, era.Name, test.Name)
			}
		}
	}
}
