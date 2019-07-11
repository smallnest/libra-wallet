document.addEventListener("DOMContentLoaded", function () {
    var pathname = document.location.pathname;
    var pathname = pathname.substr(1);
    if (pathname.length == 0) {
        pathname = "balance";
    }

    var element = document.getElementById(pathname);
    element.classList.add("active");
});