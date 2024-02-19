package main

import (
  "bufio"
  "fmt"
  "github.com/pkg/sftp"
  "golang.org/x/crypto/ssh"
  "gopkg.in/yaml.v3"
  "gosync/conf"
  "io"
  "log"
  "os"
  "path/filepath"
  "strings"
  "time"
)

/**
https://stackoverflow.com/questions/65726482/pkg-sftp-much-slower-than-linux-scp-why
*/

func publicKeyAuthFunc(rsaFile string) ssh.AuthMethod {

  key, err := os.ReadFile(rsaFile)
  if err != nil {
    log.Fatal("ssh 密钥文件读取失败", err)
  }
  // Create the Signer for this private key.
  signer, err := ssh.ParsePrivateKey(key)
  if err != nil {
    log.Fatal("ssh 关键签名失败", err)
  }
  return ssh.PublicKeys(signer)
}

func humanSize(bytes int64) string {
  total := "B"
  if bytes > 1024*1024*1024 {
    total = fmt.Sprintf("%.2fGB", float64(bytes)/(1024*1024*1024))
  } else if bytes > 1024*1024 {
    total = fmt.Sprintf("%.2fMB", float64(bytes)/(1024*1024))
  } else if bytes > 1024 {
    total = fmt.Sprintf("%.2fKB", float64(bytes)/(1024))
  } else {
    total = fmt.Sprintf("%dB", bytes)
  }
  return total
}

// 连接的配置
type ClientConfig struct {
  Host       string //ip
  Port       int    // 端口
  Username   string //用户名
  Password   string //密码
  sshClient  *ssh.Client
  sftpClient *sftp.Client
  LastResult string //最近一次运行的结果
}

func (cliConf *ClientConfig) createClient(serverConf conf.ServerConf) {
  //创建ssh登陆配置
  config := &ssh.ClientConfig{
    Timeout: time.Second,
    User:    serverConf.User,
    //Auth: []gosync.AuthMethod{gosync.Password(password)},
    Auth:            []ssh.AuthMethod{publicKeyAuthFunc(serverConf.RsaFile)},
    HostKeyCallback: ssh.InsecureIgnoreHostKey(), //这个可以, 但是不够安全
  }
  //config.Ciphers = append(config.Ciphers, "3des-cbc")
  //dial 获取ssh client
  addr := fmt.Sprintf("%s:%d", serverConf.Host, serverConf.Port)
  sshClient, err := ssh.Dial("tcp", addr, config)
  if err != nil {
    log.Fatal("创建ssh client 失败", err)
  }
  cliConf.sshClient = sshClient
  var sftpClient *sftp.Client
  //此时获取了sshClient，下面使用sshClient构建sftpClient
  sftpClient, err = sftp.NewClient(sshClient,
    sftp.UseConcurrentReads(true),
    sftp.UseConcurrentWrites(true),
    sftp.MaxConcurrentRequestsPerFile(64),
  )
  if err != nil {
    log.Fatalln("error occurred:", err)
  }
  cliConf.sftpClient = sftpClient

}

func (cliConf *ClientConfig) RunShell(shell string) string {
  var (
    session *ssh.Session
    err     error
  )
  //获取session，这个session是用来远程执行操作的
  if session, err = cliConf.sshClient.NewSession(); err != nil {
    log.Fatalln("error occurred:", err)
  }
  //执行shell
  if output, err := session.CombinedOutput(shell); err != nil {
    fmt.Println(shell)
    log.Fatalln("error occurred:", err)
  } else {
    cliConf.LastResult = string(output)
  }
  return cliConf.LastResult
}

func (cliConf *ClientConfig) Upload(srcFilePath, dstFilePath string) {
  srcFile, _ := os.Open(srcFilePath)                   //本地文件
  dstFile, _ := cliConf.sftpClient.Create(dstFilePath) //远程文件
  defer func() {
    _ = srcFile.Close()
    _ = dstFile.Close()
  }()

  stat, _ := srcFile.Stat()
  buf := make([]byte, 1024*1024*8)
  reader := bufio.NewReaderSize(srcFile, 4096*10)
  writer := bufio.NewWriterSize(dstFile, 4096*10)
  var size int64 = 0
  for {
    n, err := reader.Read(buf)
    if err != nil {
      if err != io.EOF {
        log.Fatalln("error occurred:", err)
      } else {
        break
      }
    }
    nn, _ := writer.Write(buf[:n])
    size += int64(nn)
    fmt.Printf("total=%s,current=%s,progress=%.2f%%\n", humanSize(stat.Size()), humanSize(size),
      float64(size)/float64(stat.Size())*100)
  }

  fmt.Println(cliConf.RunShell(fmt.Sprintf("ls %s", dstFilePath)))
}

func (cliConf *ClientConfig) Download(srcPath, dstPath string) {
  srcFile, err := cliConf.sftpClient.Open(srcPath) //远程
  fmt.Println(srcFile)
  if err != nil {
    fmt.Println(err.Error())
  }

  dstFile, _ := os.Create(dstPath) //本地
  defer func() {
    _ = srcFile.Close()
    _ = dstFile.Close()
  }()

  if _, err := srcFile.WriteTo(dstFile); err != nil {
    log.Fatalln("error occurred", err)
  }
  fmt.Println("文件下载完毕")
}

func getConf() *conf.Config {
  configPath := "resources/config.yaml"
  _, err := os.Stat(configPath)
  if err != nil {
    configPath = "conf/config.yaml"
  }
  _, err = os.Stat(configPath)
  if err != nil {
    log.Fatalln("config file not exist")
    return nil
  }
  f, err := os.ReadFile(configPath)
  if err != nil {
    log.Fatalln("read failed")
    return nil
  }
  yaml.Unmarshal(f, &conf.AppConf)
  return &conf.AppConf

}

func startUpload(appConf *conf.Config) {
  start := time.Now()
  fmt.Println(start)
  cliConf := new(ClientConfig)
  cliConf.createClient(appConf.Server)
  _, fileName := filepath.Split(appConf.Upload.SrcFile)
  dstFile := appConf.Upload.DstDir + fileName
  if !strings.HasSuffix(appConf.Upload.DstDir, "/") {
    dstFile = appConf.Upload.DstDir + "/" + fileName
  }
  cliConf.Upload(appConf.Upload.SrcFile, dstFile)
  end := time.Now()
  duration := end.Sub(start)
  fmt.Printf("end=%s,cost=%02d:%02d:%02d\n", end.Format("2006-01-02 15:04:05"),
    int(duration.Hours()), int(duration.Minutes())%60, int(duration.Seconds())%60)
}

func main() {
  c := getConf()
  if c == nil {
    fmt.Println("conf nil")
    time.Sleep(time.Second * 5)
    return
  }
  startUpload(c)
  time.Sleep(time.Second * 5)
  //从服务器中下载文件
  //cliConf.Download("/root/test/info.json", "./info.json")
}
