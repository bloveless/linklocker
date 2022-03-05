const addALinkButton = document.getElementById("add-a-link");
const addALinkForm = document.getElementById("add-a-link-form");

if (addALinkButton && addALinkForm) {
    addALinkButton.addEventListener('click', () => {
        if (addALinkForm.classList.contains('hidden')) {
            addALinkForm.classList.remove('hidden');
        } else {
            addALinkForm.classList.add('hidden');
        }
    });
}
