package conf

var AppConf Config

type Config struct {
	Server ServerConf
	Upload UploadConf
}
type ServerConf struct {
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	User    string `yaml:"user"`
	RsaFile string `yaml:"rsaFile"`
}

type UploadConf struct {
	SrcFile string `yaml:"srcFile"`
	DstDir  string `yaml:"dstDir"`
}
