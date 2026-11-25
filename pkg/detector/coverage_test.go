package detector_test

import (
	"testing"

	"dep_service/pkg/detector"
)

// Formats that the PDF preview used to catch with its own TypeScript regexes.
// Go is now the only source of PII rules, so each of them must be found here.
func TestFindTextMatches_FormerClientRules(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		typ   detector.EntityType
		value string
	}{
		{"phone dotted", "Резервный номер 8.916.111.22.33, рабочий", detector.EntityPhone, "8.916.111.22.33"},
		{"phone 7 without plus", "рабочий 7 495 1234567.", detector.EntityPhone, "7 495 1234567"},
		{"phone plus with space", "звонить + 7 916 123 45 67", detector.EntityPhone, "+ 7 916 123 45 67"},
		{"phone brackets", "Тел. для связи 8(916)1234567", detector.EntityPhone, "8(916)1234567"},
		{"phone spaced brackets", "звонить 8 ( 916 ) 123-45-67", detector.EntityPhone, "8 ( 916 ) 123-45-67"},
		{"phone plus any grouping", "звонить +7 9161 23 45 67", detector.EntityPhone, "+7 9161 23 45 67"},
		{"date month genitive", "г. Москва 15 марта 2024 г.", detector.EntityDate, "15 марта 2024"},
		{"date month with year word", "родился 3 апреля 1979 года", detector.EntityDate, "3 апреля 1979"},
		{"date month capitalized", "Дата: 01 Января 2000", detector.EntityDate, "01 Января 2000"},
		{"date slash", "Согласовано 12/05/2023.", detector.EntityDate, "12/05/2023"},
		{"card grouped without luhn", "карта 4276 3800 1234 5678, резервная", detector.EntityCard, "4276 3800 1234 5678"},
		{"card grouped dashes", "карта 2200-1234-5678-9012", detector.EntityCard, "2200-1234-5678-9012"},
		{"inn10 standalone", "организация 7707083893 зарегистрирована", detector.EntityINN, "7707083893"},
		{"fio accusative patronymic", "уведомили Петрову Анну Сергеевну о", detector.EntityFIO, "Петрову Анну Сергеевну"},
		{"fio instrumental archaic", "с Ивановой Марией Петровною", detector.EntityFIO, "Ивановой Марией Петровною"},
		{"fio initials after surname", "Директор Сидоров П. А. действует", detector.EntityFIO, "Сидоров П. А."},
		{"fio initials before surname", "Согласовано: А. В. Кузнецов, 2023", detector.EntityFIO, "А. В. Кузнецов"},
		{"address street chain", "Проживает: г. Казань, ул. Баумана, д. 17, кв. 3", detector.EntityAddress, "г. Казань, ул. Баумана, д. 17, кв. 3"},
		{"address street first", "объект: ул. Новая Басманная, д. 5/2, корп. 1", detector.EntityAddress, "ул. Новая Басманная, д. 5/2, корп. 1"},
		{"address stops at flat number", "г. Москва, ул. Ленина, д. 5, кв. 12 Проживает", detector.EntityAddress, "г. Москва, ул. Ленина, д. 5, кв. 12"},
		{"passport 4-6", "паспорт серия 4510 № 123456, выдан", detector.EntityPassport, "4510 № 123456"},
		{"passport series word number", "паспорт серия 4014 номер 382910, выдан", detector.EntityPassport, "4014 номер 382910"},
		{"passport split series word number", "Паспорт: серия 40 14 номер 382910", detector.EntityPassport, "40 14 номер 382910"},
		{"address postal registration", "зарегистрирован по адресу: 197374, г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48", detector.EntityAddress, "197374, г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48"},
		{"address after place of residence", "гражданка, зарегистрированная по месту жительства по адресу: 197374, г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48, паспорт", detector.EntityAddress, "197374, г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48"},
		{"address ends at closing bracket", "ООО (ИНН 7814987650, адрес: 197374, г. Санкт-Петербург, ул. Савушкина, д. 126, лит. Б, оф. 302) на обработку моих данных", detector.EntityAddress, "197374, г. Санкт-Петербург, ул. Савушкина, д. 126, лит. Б, оф. 302"},
		{"address street without postal code", "г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48", detector.EntityAddress, "г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48"},
		{"passport 2-2-6", "Паспорт представителя 45 10 654321.", detector.EntityPassport, "45 10 654321"},
		{"snils", "СНИЛС 112-233-445 95, ИНН", detector.EntitySNILS, "112-233-445 95"},
		{"email", "Email: ivanov.ivan@example.ru", detector.EntityEmail, "ivanov.ivan@example.ru"},
		{"date dmy", "выдан 12.05.2015, код", detector.EntityDate, "12.05.2015"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matches := detector.FindTextMatches(tc.text)
			for _, m := range matches {
				if m.EntityType == tc.typ && m.Value == tc.value {
					if tc.text[m.Start:m.End] != m.Value {
						t.Fatalf("offsets %d..%d point to %q, value is %q", m.Start, m.End, tc.text[m.Start:m.End], m.Value)
					}
					return
				}
			}
			t.Errorf("expected %s %q in %q, got %+v", tc.typ, tc.value, tc.text, matches)
		})
	}
}

// The new rules must not turn ordinary numbers and words into PII.
func TestFindTextMatches_FormerClientRulesNegatives(t *testing.T) {
	cases := []struct {
		name string
		text string
		typ  detector.EntityType
	}{
		{"contract number is not inn", "Номер договора 1234567890.", detector.EntityINN},
		{"date followed by number is not phone", "от 8.12.2023 1234 руб.", detector.EntityPhone},
		{"impossible day", "32 марта 2020", detector.EntityDate},
		{"impossible slash date", "45/13/2020", detector.EntityDate},
		{"city alone is not address", "г. Москва 15 марта 2024 г.", detector.EntityAddress},
		{"part of longer digit run is not inn", "счёт 77070838931", detector.EntityINN},
		{"ungrouped 16 digits without luhn is not card", "код 5469380087654321", detector.EntityCard},
		{"grouped digits without card prefix are not card", "партия 1234 5678 9012 3456", detector.EntityCard},
		{"pdf drawing operands are not phone", "q 8 595.276 842.0 re f Q 7 100.5 200.25 m", detector.EntityPhone},
		{"pdf integer operands are not phone", "7 612 792 0 0 cm", detector.EntityPhone},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, m := range detector.FindTextMatches(tc.text) {
				if m.EntityType == tc.typ {
					t.Errorf("unexpected %s %q in %q", m.EntityType, m.Value, tc.text)
				}
			}
		})
	}
}

func TestValidateINN10(t *testing.T) {
	if !detector.ValidateINN10("7707083893") {
		t.Error("7707083893 must be a valid 10-digit INN")
	}
	if detector.ValidateINN10("1234567890") {
		t.Error("1234567890 must fail the INN checksum")
	}
	if detector.ValidateINN10("123") {
		t.Error("short input must be rejected")
	}
}
