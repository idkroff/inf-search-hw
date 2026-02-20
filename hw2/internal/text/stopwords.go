package text

func StopWords(language string) []string {
	switch language {
	case "english":
		return englishStopWords
	case "russian":
		return russianStopWords
	default:
		return nil
	}
}

var englishStopWords = []string{
	"a", "an", "the", "and", "or", "but", "not", "nor", "so", "yet",
	"both", "either", "neither", "for", "although", "because", "since",
	"unless", "while", "after", "as", "at", "be", "been", "being",
	"by", "can", "could", "did", "do", "does", "during", "each",
	"few", "from", "further", "get", "got", "had", "has", "have",
	"having", "he", "her", "here", "him", "his", "how", "i", "if",
	"in", "into", "is", "it", "its", "itself", "just", "ll", "me",
	"might", "more", "most", "must", "my", "myself", "no", "now",
	"of", "off", "on", "once", "only", "other", "our", "ours",
	"ourselves", "out", "over", "own", "re", "s", "same", "she",
	"should", "some", "such", "t", "than", "that", "their", "theirs",
	"them", "themselves", "then", "there", "these", "they", "this",
	"those", "through", "to", "too", "until", "up", "us", "ve",
	"very", "was", "we", "were", "what", "when", "where", "which",
	"while", "who", "whom", "why", "will", "with", "would", "you",
	"your", "yours", "yourself", "yourselves", "shall", "may",
	"also", "about", "above", "between", "below", "before", "under",
	"again", "then", "any", "all", "am",
}

var russianStopWords = []string{
	"и", "в", "во", "не", "что", "он", "на", "я", "с", "со", "как",
	"а", "то", "все", "она", "так", "его", "но", "да", "ты", "к",
	"у", "же", "вы", "за", "бы", "по", "только", "ее", "мне", "было",
	"вот", "от", "меня", "еще", "нет", "о", "из", "ему", "теперь",
	"когда", "даже", "ну", "вдруг", "ли", "если", "уже", "или",
	"ни", "быть", "был", "него", "до", "вас", "нибудь", "опять",
	"уж", "вам", "сказал", "ведь", "там", "потом", "себя", "ничего",
	"ей", "может", "они", "тут", "где", "есть", "надо", "ней", "для",
	"мы", "тебя", "их", "чем", "была", "сам", "чтоб", "без", "будто",
	"человек", "чего", "раз", "тоже", "себе", "под", "будет", "ж",
	"тогда", "кто", "этот", "того", "потому", "этого", "какой", "совсем",
	"этой", "одного", "при", "об", "им", "три", "хоть", "после", "над",
	"больше", "тот", "через", "эти", "нас", "про", "всего", "них", "какая",
	"много", "разве", "сказала", "три", "эту",
}
