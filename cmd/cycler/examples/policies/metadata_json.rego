package cycler

act := true {
    re_match(`.*release/.*/metadata\.json`, input.attr.Name)
}
