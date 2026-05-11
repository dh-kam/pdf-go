package renderer

func (e *Evaluator) syncTextMatrixState(textMatrix [6]float64) {
	e.textMatrix = textMatrix
	e.graphics.textMatrix = textMatrix
	e.graphics.currentState.SetTextMatrix(textMatrix)
	e.textCurrentValid = false
	e.textUserCurrentX = textMatrix[4]
	e.textUserCurrentY = textMatrix[5]
	e.textUserCurrentValid = true
}

func (e *Evaluator) syncTextMatricesState(textMatrix, textLineMatrix [6]float64) {
	e.textLineMatrix = textLineMatrix
	e.graphics.textLine = textLineMatrix
	e.syncTextMatrixState(textMatrix)
}

func (e *Evaluator) syncPopplerTextBase(textMatrix [6]float64, lineX, lineY float64) {
	e.textBaseMatrix = textMatrix
	e.textLineX = lineX
	e.textLineY = lineY
	e.textUserCurrentX = textMatrix[0]*lineX + textMatrix[2]*lineY + textMatrix[4]
	e.textUserCurrentY = textMatrix[1]*lineX + textMatrix[3]*lineY + textMatrix[5]
	e.textUserCurrentValid = true
}
