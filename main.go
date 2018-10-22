package main

import (
  "os"
  "bytes"
  "io"
  "io/ioutil"
  "fmt"
  "strings"
  "context"
  "archive/tar"
  "regexp"

  "golang.org/x/oauth2"
  "github.com/shurcooL/githubv4"
  "gopkg.in/src-d/go-git.v4"
  "github.com/docker/docker/api/types"
  "github.com/docker/docker/client"
)

type repository struct {
  Name string
  SshUrl string
  Url string
}

var repoQuery struct {
  Organization struct {
    Repositories struct {
      Nodes []repository
      PageInfo struct {
        EndCursor githubv4.String
        HasNextPage bool
      }
    } `graphql:"repositories(first: 100, after: $reposCursor)"`
  } `graphql:"organization(login: $org)"`
}

func main() {
  const reposPath = "/tmp/wizecat/repos"
  // Begin: for docker image build package
  ctx := context.Background()
  cli, err := client.NewEnvClient()
  if err != nil {
    fmt.Println(err)
    panic(err)
  }
  buf := new(bytes.Buffer)
  tw := tar.NewWriter(buf)
  defer tw.Close()
  // End: for docker image build package

  // Begin: for authentication package
  src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("CORGI_TOKEN")})
  httpClient := oauth2.NewClient(ctx, src)
  // End: for authentication package


  // Begin: for clients package (in the --lives option for spawning clients)
  githubClient := githubv4.NewClient(httpClient)
  // end: for clients package (in the --lives option for spawning clients) (SABS: don't know where to put it yet)

  // Setting query variables
  var allRepos []repository;
  variables := map[string]interface{}{
    "org": githubv4.String("wizeline"),
    "reposCursor":  (*githubv4.String)(nil), // Null after argument to get first page.
  }

  // iterate through repos
  for {
    err := githubClient.Query(ctx, &repoQuery, variables)
    if err != nil {
      fmt.Println(err)
      break
    }

    // Appending more that one elements to the allRepos slice
    allRepos = append(allRepos, repoQuery.Organization.Repositories.Nodes...)
    // If there's no next page beak
    if !repoQuery.Organization.Repositories.PageInfo.HasNextPage {
      break
    }
    // Place the next cursor for pagination
    variables["reposCursor"] = githubv4.NewString(repoQuery.Organization.Repositories.PageInfo.EndCursor)
  }

  var cloned, notCloned , dockerFound, imageBuilt, errorBuilts int
  // Cloning of the repositories
  for i := 0; i <= len(allRepos) - 1; i++ {
    // if i % 40 != 0 {
    //   continue
    // }
    repo := allRepos[i].SshUrl
    tmpRepoPath := fmt.Sprintf("%s/%s", reposPath, allRepos[i].Name)
    // Cloning the repo in the temporary path
    _, err := git.PlainClone(tmpRepoPath, false, &git.CloneOptions{
      URL: repo,
    })
    if err != nil {
      notCloned++
      fmt.Println("Repository cloning ERROR: ", err)
      os.RemoveAll(tmpRepoPath)
      continue
      // fmt.Printf("Removed %s directory, continuing...\n\n\n", tmpRepoPath)
    } else {
      // fmt.Printf("Cloned %s\n", repo)
      cloned++
    }

    // WORKING WITH DOCKER SDK

    // Preparing to build image
    // Opening and Reading dockerfile 
    dockerFilePath := fmt.Sprintf("%s/Dockerfile", tmpRepoPath)
    corgiLogFilePath := fmt.Sprintf("%s/corgi.log", tmpRepoPath)
    dockerFileReader, err := os.Open(dockerFilePath)
    if err != nil {
      // fmt.Println("Unable to OPEN Dockerfile at: ", dockerFilePath)
      // fmt.Println(err)
      os.RemoveAll(tmpRepoPath)
      continue
    }
    readDockerFile, err := ioutil.ReadAll(dockerFileReader)
    if err != nil {
      // fmt.Println("Unable to READ Dockerfile at: ", dockerFilePath)
      // fmt.Println(err)
      os.RemoveAll(tmpRepoPath)
      continue
    }

    tarHeader := &tar.Header{
      Name: allRepos[i].Name,
      Size: int64(len(readDockerFile)),
    }
    err = tw.WriteHeader(tarHeader)
    if err != nil {
      fmt.Println("Unable to WRITE TAR HEADER at: ", dockerFilePath)
      fmt.Println(err)
      os.RemoveAll(tmpRepoPath)
      continue
    }
    _, err = tw.Write(readDockerFile)
    if err != nil {
      fmt.Println("Unable to WRITE TAR FILE at: ", dockerFilePath)
      fmt.Println(err)
      os.RemoveAll(tmpRepoPath)
      continue
    }
    dockerFound++
    dockerFileTarReader := bytes.NewReader(buf.Bytes())

    // Building docker image
    // fmt.Println("Building image: ", dockerFilePath)
    imageBuildResponse, err := cli.ImageBuild(
      ctx,
      dockerFileTarReader,
      types.ImageBuildOptions{
        Context: dockerFileTarReader,
        Dockerfile: allRepos[i].Name,
        Remove: true,
    })
    if err != nil {
      fmt.Println("Unable to Build Docker image")
      fmt.Println(err)
      os.RemoveAll(tmpRepoPath)
      continue
    }
    defer imageBuildResponse.Body.Close()
    corgiLog, err := os.OpenFile(corgiLogFilePath, os.O_RDWR|os.O_CREATE, 0666)
    if err != nil {
      fmt.Println("Unable to Open CORGI log file")
      fmt.Println(err)
      os.RemoveAll(tmpRepoPath)
      break
    }
    defer corgiLog.Close()
    // fmt.Println("Writing CORGI log to: ", corgiLogFilePath)
    // Check the reponse from the docker daemon
    bytesCopied, err := io.Copy(corgiLog, imageBuildResponse.Body)
    if err != nil {
      fmt.Println("Unable to read image build response")
      fmt.Println(err)
      os.RemoveAll(tmpRepoPath)
      continue
    }
    // fmt.Println("Wrote CORGI log to: ", corgiLogFilePath)
    // Don't forget to return the pointer to the beginnig of the file
    corgiLog.Seek(0, 0)
    corgiSlice := make([]byte, bytesCopied)
    _, err = corgiLog.Read(corgiSlice)
    if err != nil {
      fmt.Println("Unable to read CORGO file")
      fmt.Println(err)
      os.RemoveAll(tmpRepoPath)
      continue
    }
    logString := string(corgiSlice)
    if strings.Contains(logString, "errorDetail") {
      errorBuilts++
      fmt.Println("===========Image Build error===========")
      fmt.Println("Wrote CORGI log to: ", corgiLogFilePath)
    }
    if strings.Contains(logString, "aux") {
      re := regexp.MustCompile("sha256\\:(.*?)\"")
      match := re.FindStringSubmatch(logString)
      fmt.Println("===========Image ID===========")
      fmt.Println("CORGI log: ", corgiLogFilePath)
      fmt.Println(match[1])
      imageBuilt++
    }
  }

  // Results of the cloning
  fmt.Printf("\nCORGIS EVERYWHERE... Of %d repositories\n", len(allRepos))
  fmt.Printf("%d were cloned, and of those I found %d Dockerfiles\n", cloned, dockerFound);
  fmt.Printf("%d not were cloned because of an error\n", notCloned);
  fmt.Printf("I Built %d images\n", imageBuilt);
  fmt.Printf("There were %d images not built\n", errorBuilts);
}
