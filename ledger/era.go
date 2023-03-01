package ledger

type Era struct {
	Id   uint8
	Name string
}

var eras = map[uint8]Era{
	ERA_ID_BYRON: Era{
		Id:   ERA_ID_BYRON,
		Name: "Byron",
	},
	ERA_ID_SHELLEY: Era{
		Id:   ERA_ID_SHELLEY,
		Name: "Shelley",
	},
	ERA_ID_ALLEGRA: Era{
		Id:   ERA_ID_ALLEGRA,
		Name: "Allegra",
	},
	ERA_ID_MARY: Era{
		Id:   ERA_ID_MARY,
		Name: "Mary",
	},
	ERA_ID_ALONZO: Era{
		Id:   ERA_ID_ALONZO,
		Name: "Alonzo",
	},
	ERA_ID_BABBAGE: Era{
		Id:   ERA_ID_BABBAGE,
		Name: "Babbage",
	},
}

func GetEraById(eraId uint8) *Era {
	era, ok := eras[eraId]
	if !ok {
		return nil
	}
	return &era
}
