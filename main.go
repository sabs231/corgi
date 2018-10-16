package main

import (
  "os"
  "io/ioutil"
  "fmt"
  "strings"
  "context"
  "golang.org/x/oauth2"
  "github.com/shurcooL/githubv4"
  "gopkg.in/src-d/go-git.v4"
)

// GITHUB_TOKEN: f39035c4a93b4580abe9aaed1c8ca29bb5bdb98d

var query struct {
  Viewer struct {
    Login githubv4.String
    CreatedAt githubv4.DateTime
  }
}

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
  // Begin: for authentication package
  src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "f39035c4a93b4580abe9aaed1c8ca29bb5bdb98d"})
  httpClient := oauth2.NewClient(context.Background(), src)
  // End: for authentication package

  // Begin: for clients package (in the --lives option for spawning clients)
  client := githubv4.NewClient(httpClient)
  // end: for clients package (in the --lives option for spawning clients) (SABS: don't know where to put it yet)

  // Setting query variables
  var allRepos []repository;
  variables := map[string]interface{}{
    "org": githubv4.String("wizeline"),
    "reposCursor":  (*githubv4.String)(nil), // Null after argument to get first page.
  }

  // iterate through repos
  for {
    err := client.Query(context.Background(), &repoQuery, variables)
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

  var cloned, notCloned , dockerFound int
  // Cloning of the repositories
  for i := 0; i <= len(allRepos) - 1; i++ {
    repo := allRepos[i].SshUrl
    tmpRepoPath := fmt.Sprintf("%s/%s", reposPath, allRepos[i].Name)
    // Cloning the repo in the temporary path
    _, err := git.PlainClone(tmpRepoPath, false, &git.CloneOptions{
      URL: repo,
    })
    if err != nil {
      notCloned++
      // fmt.Println("Repository cloning ERROR: ", err)
      os.RemoveAll(tmpRepoPath)
      continue
      // fmt.Printf("Removed %s directory, continuing...\n\n\n", tmpRepoPath)
    } else {
      // fmt.Printf("Cloned %s\n", repo)
      cloned++
    }

    // Reading tmp repo directory
    files, err := ioutil.ReadDir(tmpRepoPath)
    if err != nil {
      fmt.Println("Error reading directory: ", tmpRepoPath)
      break
    }
    var foundDocker bool
    for _, file := range files {
      if strings.Compare(file.Name(), "Dockerfile") == 0 {
        dockerFound++
        foundDocker = true
        fmt.Printf("FOUND Dockerfile! In directory: %s\n", tmpRepoPath)
        break
      }
    }
    // if no dockerfile found why do I even bother having that directory
    if !foundDocker {
      os.RemoveAll(tmpRepoPath)
    }
  }

  // Results of the cloning
  fmt.Printf("\nMEEEOW... Of %d repositories\n", len(allRepos))
  fmt.Printf("%d were cloned, and of those I found %d Dockerfiles\n", cloned, dockerFound);
  fmt.Printf("%d not were cloned because of an error\n", notCloned);
}
