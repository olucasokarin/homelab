package bot

var Season3Map = map[string]string{
	// Serial 018: Galaxy 4
	"018-01": "S03E01", "018-02": "S03E02", "018-03": "S03E03", "018-04": "S03E04",

	// Serial 019: Mission to the Unknown
	"019-01": "S03E05",

	// Serial 020: The Myth Makers
	"020-01": "S03E06", "020-02": "S03E07", "020-03": "S03E08", "020-04": "S03E09",

	// Serial 021: The Daleks' Master Plan
	"021-01": "S03E10", "021-02": "S03E11", "021-03": "S03E12", "021-04": "S03E13",
	"021-05": "S03E14", "021-06": "S03E15", "021-07": "S03E16", "021-08": "S03E17",
	"021-09": "S03E18", "021-10": "S03E19", "021-11": "S03E20", "021-12": "S03E21",

	// Serial 022: The Massacre
	"022-01": "S03E22", "022-02": "S03E23", "022-03": "S03E24", "022-04": "S03E25",

	// Serial 023: The Ark
	"023-01": "S03E26", "023-02": "S03E27", "023-03": "S03E28", "023-04": "S03E29",

	// Serial 024: The Celestial Toymaker
	"024-01": "S03E30", "024-02": "S03E31", "024-03": "S03E32", "024-04": "S03E33",

	// Serial 025: The Gunfighters
	"025-01": "S03E34", "025-02": "S03E35", "025-03": "S03E36", "025-04": "S03E37",

	// Serial 026: The Savages
	"026-01": "S03E38", "026-02": "S03E39", "026-03": "S03E40", "026-04": "S03E41",

	// Serial 027: The War Machines
	"027-01": "S03E42", "027-02": "S03E43", "027-03": "S03E44", "027-04": "S03E45",
}

func applyRenamingRules(fileName string, channelTitle string) string {
	// Regra específica para UW Clássica
	if channelTitle == "UW Clássica" {
		// Padrão esperado: "021-01.O Nome.mkv"
		// Verificamos se começa com 6 caracteres (ex: 021-01) seguidos de um ponto ou traço
		if len(fileName) > 6 {
			prefix := fileName[:6]
			if newPrefix, ok := Season3Map[prefix]; ok {
				// Substituímos o prefixo antigo pelo novo padrão: "Doctor Who SXXEXX"
				// Preservamos o restante do nome (ex: .The Nightmare Begins (Recon).mkv)
				return "Doctor Who " + newPrefix + fileName[6:]
			}
		}
	}

	// Se não for do canal ou não estiver no mapa, retorna o nome original
	return fileName
}
