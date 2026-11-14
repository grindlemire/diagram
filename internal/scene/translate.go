package scene

// Translate moves items by (dx, dy) in place.
func Translate(items []Item, dx, dy float64) {
	if dx == 0 && dy == 0 {
		return
	}
	for _, it := range items {
		switch v := it.(type) {
		case *Group:
			Translate(v.Items, dx, dy)
		case *Rect:
			v.X += dx
			v.Y += dy
		case *Ellipse:
			v.CX += dx
			v.CY += dy
		case *Shape:
			for i := range v.Path {
				for j := range v.Path[i].Pts {
					v.Path[i].Pts[j].X += dx
					v.Path[i].Pts[j].Y += dy
				}
			}
		case *Text:
			v.X += dx
			v.Y += dy
		case *Image:
			v.X += dx
			v.Y += dy
		}
	}
}
