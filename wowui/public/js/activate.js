var orgId;
var userName;

var registration = {
    "username": userName,
    "orgid": orgId
}

function inputFocus(i) {
    if (i.value == i.defaultValue) {
        i.value = "";
        i.style.color = "#000";
    }
}

function inputBlur(i) {
    if (i.value == "") {
        i.value = i.defaultValue;
        i.style.color = "#888";
    }
}

function activateURL() {
    var activationURL;
    orgId = document.getElementById("orgID").value;
    submitOK = true;

    if (orgId === "b304dv") {
        alert("Organization Name could not be empty");
        submitOK = false;
    }

    if (submitOK) {

        activationURL = "https://" + orgId + ".internetofthings.ibmcloud.com/api/v0002/blockchain/activate?code=" + orgId;
    }

    return activationURL;
}

function activateRedirectURL() {
    window.location = activateURL();
}

function configURL() {
    var configURL;

    orgId = document.getElementById("orgID").value;
    username = document.getElementById("uname").value;

    submitOK = true;

    if (orgId === "b304dv") {
        alert("Organization Name could not be empty");
        submitOK = false;
    }

    if (orgId === "") {
        alert("User Name could not be empty");
        submitOK = false;
    }

    if (submitOK) {

        activationURL = "https://" + orgId + ".internetofthings.ibmcloud.com/dashboard/#/config-v2";

        registration.username = userName;
        registration.orgid = orgId;

        $.ajax({
            url: "https://rahul-gupta.mybluemix.net/register",
            type: "POST",
            dataType: "json",
            contentType: "application/json; charset=utf-8",
            data: JSON.stringify(registration),
            success: function(response) {
                $('#textarea-log').val(JSON.stringify(response));
            },
            error: function(xhr, status, error) {
                console.error(registration);
                console.error("Error while user registration");
            }
        });
    }

    return activationURL;
}

function configRedirectURL() {
    window.location = configURL();
}