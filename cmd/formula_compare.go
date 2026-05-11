package main

import (
  "fmt"
  "image"
  "image/color"
  "image/draw"

  "github.com/dh-kam/pdf-go/internal/infrastructure/canvas"
  xdraw "golang.org/x/image/draw"
  "golang.org/x/image/math/f64"
)

func countNZ(img *image.RGBA) int {
  c:=0
  for y:=img.Bounds().Min.Y;y<img.Bounds().Max.Y;y++ {
    for x:=img.Bounds().Min.X;x<img.Bounds().Max.X;x++ { if img.RGBAAt(x,y).A!=0 {c++}}
  }
  return c
}

func main() {
  src:=image.NewRGBA(image.Rect(0,0,16,16))
  for y:=0;y<16;y++{for x:=0;x<16;x++{src.SetRGBA(x,y,color.RGBA{255,0,0,255})}}
  canv:=canvas.NewImageCanvas(image.Rect(0,0,8,8)).(*canvas.ImageCanvas)
  m := [6]float64{8,0,0,8,0,0}
  canv.Transform(m)

  _ = canv.DrawImageWithPhase(src,0,0,16,16,false,0.5,0.5)
  cur := canv.Image().(*image.RGBA)

  p00x,p00y := 0.0,0.0
  p10x,p10y := 16.0,0.0
  p01x,p01y := 0.0,16.0
  p00x,p00y = m[0]*p00x+m[2]*p00y+m[4], m[1]*p00x+m[3]*p00y+m[5]
  p10x,p10y = m[0]*p10x+m[2]*p10y+m[4], m[1]*p10x+m[3]*p10y+m[5]
  p01x,p01y = m[0]*p01x+m[2]*p01y+m[4], m[1]*p01x+m[3]*p01y+m[5]

  scaleX := (p10x - p00x) / 16.0
  scaleY := (p10y - p00y) / 16.0
  shearX := (p01x - p00x) / 16.0
  shearY := (p01y - p00y) / 16.0

  old := image.NewRGBA(canv.Bounds())
  tOld := f64.Aff3{scaleX, shearX, p00x + 0.5, -scaleY, -shearY, 8 - p00y + 0.5}
  xdraw.NearestNeighbor.Transform(old,tOld,src,src.Bounds(),draw.Src,nil)

  nw := image.NewRGBA(canv.Bounds())
  tNew := f64.Aff3{scaleX, shearX, p00x + scaleX*0.5 + shearX*0.5, -scaleY, -shearY, 8 - p00y - (scaleY*0.5 + shearY*0.5)}
  xdraw.NearestNeighbor.Transform(nw,tNew,src,src.Bounds(),draw.Src,nil)

  fmt.Println("cur nz",countNZ(cur),"old nz",countNZ(old),"new nz",countNZ(nw))
}
