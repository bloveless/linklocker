const addALinkButton = document.getElementById("add-a-link");
const addALinkForm = document.getElementById("add-a-link-form");

if (addALinkButton && addALinkForm) {
    addALinkButton.addEventListener('click', function () {
        if (addALinkForm.classList.contains('hidden')) {
            addALinkForm.classList.remove('hidden');
        } else {
            addALinkForm.classList.add('hidden');
        }
    });
}

const refreshScreenshotButtons = document.getElementsByClassName("refresh-link");

Array.from(refreshScreenshotButtons).forEach(function(element) {
    element.addEventListener('click', function(event) {
        event.preventDefault();

        const linkId = element.dataset.linkId;

        console.log("Refresh link id", linkId);
    });
});

const editScreenshotButtons = document.getElementsByClassName("edit-link");

Array.from(editScreenshotButtons).forEach(function(element) {
    element.addEventListener('click', function(event) {
        event.preventDefault();

        const linkId = element.dataset.linkId;

        console.log("Edit link id", linkId);
    });
})
