package mpkg

// 创建23号的Ark
func ArkMessage(idiom *IdiomData) *Ark {
	return &Ark{
		TemplateID: 23,
		KV:         createArkKvArray(idiom),
	}
}

// 创建Ark需要的ArkKV数组
func createArkKvArray(idiom *IdiomData) []*ArkKV {
	akvArray := make([]*ArkKV, 3)
	akvArray[0] = &ArkKV{
		Key:   "#DESC#",
		Value: "描述",
	}
	akvArray[1] = &ArkKV{
		Key:   "#PROMPT#",
		Value: "#PROMPT#",
	}
	akvArray[2] = &ArkKV{
		Key: "#LIST#",
		Obj: createArkObjArray(idiom),
	}
	return akvArray
}

// 创建ArkKV需要的ArkObj数组
func createArkObjArray(idiom *IdiomData) []*ArkObj {
	objectArray := []*ArkObj{
		{
			[]*ArkObjKV{
				{
					Key:   "desc",
					Value: idiom.Word + " " + idiom.Pinyin,
				},
			},
		},
		{
			[]*ArkObjKV{
				{
					Key:   "desc",
					Value: "出处：" + idiom.Derivation,
				},
			},
		},
		{
			[]*ArkObjKV{
				{
					Key:   "desc",
					Value: "解释：" + idiom.Explanation,
				},
			},
		},
	}
	return objectArray
}
