package tokenizer

func byteTables() ([256]rune, map[rune]byte) {
	var byteToRune [256]rune
	runeToByte := make(map[rune]byte, 256)
	values := make([]int, 0, 256)

	for value := int('!'); value <= int('~'); value++ {
		values = append(values, value)
	}

	for value := int('¡'); value <= int('¬'); value++ {
		values = append(values, value)
	}

	for value := int('®'); value <= int('ÿ'); value++ {
		values = append(values, value)
	}

	selected := make(map[int]bool, len(values))

	for _, value := range values {
		selected[value] = true
	}

	runes := append([]int(nil), values...)
	next := 0

	for value := 0; value < 256; value++ {
		if selected[value] {
			continue
		}

		values = append(values, value)
		runes = append(runes, 256+next)
		next++
	}

	for index, value := range values {
		encodedRune := rune(runes[index])
		byteToRune[byte(value)] = encodedRune
		runeToByte[encodedRune] = byte(value)
	}

	return byteToRune, runeToByte
}
