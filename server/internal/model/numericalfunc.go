package model

type NumericalFunctionType int32

const (
	NumericalFunctionTypeUnknown               NumericalFunctionType = 0
	NumericalFunctionTypeLinear                NumericalFunctionType = 1
	NumericalFunctionTypeMonomial              NumericalFunctionType = 2
	NumericalFunctionTypeDuplexLinear          NumericalFunctionType = 3
	NumericalFunctionTypeLinearPermil          NumericalFunctionType = 4
	NumericalFunctionTypePolynomialThird       NumericalFunctionType = 5
	NumericalFunctionTypePolynomialThirdPermil NumericalFunctionType = 6
	NumericalFunctionTypePartsMainOption       NumericalFunctionType = 7
)
