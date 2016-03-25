try:
    import traceback
    import sublime
    print("new file")
    v = sublime.active_window().new_file()
    print("running command")
    v.run_command("test_text")
    print("command ran")
    assert v.substr(sublime.Region(0, v.size())) == "hello"
    v.run_command("undo")
    print(v.sel()[0])
    assert v.sel()[0] == (0, 0)
    v = sublime.active_window().active_view()
    sublime.active_window().run_command("test_window")
    assert v.substr(sublime.Region(0, v.size())) == "window hello"
except:
    traceback.print_exc()
    raise
