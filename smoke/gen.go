package main

import ("archive/tar"; "os"; "fmt")

func layer(file string, hdrs []*tar.Header, bodies []string) {
	f, _ := os.Create(file)
	tw := tar.NewWriter(f)
	for i, h := range hdrs {
		tw.WriteHeader(h)
		if h.Size > 0 { tw.Write([]byte(bodies[i])) }
	}
	tw.Close(); f.Close()
}

func main() {
	layer("smoke/l1.tar", []*tar.Header{
		{Name:"app/run", Typeflag:tar.TypeReg, Mode:0644, Size:5},
		{Name:"etc", Typeflag:tar.TypeDir, Mode:0755},
		{Name:"etc/old", Typeflag:tar.TypeReg, Mode:0644, Size:3},
		{Name:"rel", Typeflag:tar.TypeSymlink, Linkname:"/app/run"},
	}, []string{"hello","","old",""})
	layer("smoke/l2.tar", []*tar.Header{
		{Name:"etc/.wh..wh..opq", Typeflag:tar.TypeReg, Mode:0644},
		{Name:"etc/new", Typeflag:tar.TypeReg, Mode:0644, Size:3},
		{Name:"app/run", Typeflag:tar.TypeReg, Mode:0644, Size:5},
	}, []string{"","new","hello"})
	fmt.Println("done")
}
