package id.web.quakealert.ui.addsensor

import id.web.quakealert.ui.common.ErrorAction
import id.web.quakealert.ui.common.ErrorCopy

/**
 * Every word the add-a-sensor wizard can put on screen, in one file.
 *
 * The state carries situations ([WizardFailure], [DetailsError], [LinkError]), never
 * sentences, so this mapper is the only place a failure becomes text. That is what
 * makes the house rule enforceable rather than aspirational: there is no path from a
 * server body, an OkHttp message or an exception name to the card, because the screen
 * has nothing to render but these constants.
 *
 * The rules are the ones [id.web.quakealert.ui.common.errorCopy] documents: name what
 * failed in the user's terms, say what still works, offer at most one action that can
 * actually help, and never show a status code, a host, a socket state or a class name.
 * Two wizard-specific additions:
 *  - never name the sensor's own setup network or its configuration page; the user
 *    joined it through a system dialog and has no other handle on it;
 *  - never name a control by its icon ("tap the pencil"), because the icon may move.
 */

/** Card title for each step. Welcome carries its title in the body art instead. */
internal fun AddSensorWizardStep.headline(): String = when (this) {
    AddSensorWizardStep.WELCOME -> ""
    AddSensorWizardStep.LOCATION -> "Where would you like to provision this sensor?"
    AddSensorWizardStep.CREDENTIALS -> "Station ID and Credentials Initialization"
    AddSensorWizardStep.WLAN -> "WLAN Setup"
    AddSensorWizardStep.FINISHING -> "Finishing"
    AddSensorWizardStep.RATE_LIMIT -> "Can't add new sensor"
}

/** The quiet paragraph under each step body. */
internal fun AddSensorWizardStep.helperText(): String = when (this) {
    AddSensorWizardStep.WELCOME -> ""
    AddSensorWizardStep.LOCATION ->
        "Drag the map to place your sensor. You can use your exact position or shift " +
            "the pin a few meters for privacy. The exact position gives the best " +
            "detection accuracy."
    AddSensorWizardStep.CREDENTIALS ->
        "A unique signature built into this sensor. Every reading it sends is signed " +
            "with it, so QuakeAlert knows the data genuinely came from your device, not " +
            "an imposter.\n\nShown now because it can never be viewed again after setup."
    AddSensorWizardStep.WLAN ->
        "If a network is not showing, place your sensor closer to your router. Make " +
            "sure you enter the right password, you can't change it later."
    AddSensorWizardStep.FINISHING ->
        "Thank you for your contribution to QuakeAlert Network. After this step, your " +
            "sensor will be labelled as pending for a few days. For more information, " +
            "see Sensors Help."
    AddSensorWizardStep.RATE_LIMIT -> ""
}

/**
 * Which slot of the five-segment indicator the screen occupies. The rate limit
 * replaces the flow rather than advancing it; parking its segment at the end keeps
 * the animation monotonic.
 */
internal fun AddSensorWizardStep.indicatorIndex(): Int = when (this) {
    AddSensorWizardStep.WELCOME -> 0
    AddSensorWizardStep.LOCATION -> 1
    AddSensorWizardStep.CREDENTIALS -> 2
    AddSensorWizardStep.WLAN -> 3
    AddSensorWizardStep.FINISHING -> 4
    AddSensorWizardStep.RATE_LIMIT -> 4
}

/**
 * The failure panel's copy.
 *
 * [ErrorAction.RETRY] means the step's own action can be pressed again and might work
 * this time; [ErrorAction.NONE] means nothing on this card will change the outcome, so
 * the panel offers no false hope.
 */
internal fun failureCopy(failure: WizardFailure): ErrorCopy = when (failure) {
    WizardFailure.OFFLINE -> ErrorCopy(
        title = "You are offline",
        message = "QuakeAlert cannot reach the alert network to register this sensor. " +
            "Reconnect, then try again.",
        action = ErrorAction.RETRY
    )

    WizardFailure.REGISTER_REJECTED -> ErrorCopy(
        title = "Could not register the sensor",
        message = "The alert network could not register this sensor just now. " +
            "Nothing was saved, so it is safe to try again.",
        action = ErrorAction.RETRY
    )

    WizardFailure.SENSOR_NOT_JOINED -> ErrorCopy(
        title = "Not connected to the sensor",
        message = "Your phone is not connected to the sensor yet. Try again and accept " +
            "the connection request when your phone asks for it.",
        action = ErrorAction.RETRY
    )

    WizardFailure.SENSOR_NOT_ANSWERING -> ErrorCopy(
        title = "The sensor is not answering",
        message = "The sensor did not respond. Check that it is powered on and close " +
            "to your phone, then try again.",
        action = ErrorAction.RETRY
    )

    WizardFailure.SETTINGS_NOT_ACCEPTED -> ErrorCopy(
        title = "The sensor did not accept the setup",
        message = "The sensor did not keep these settings. Keep it powered on and " +
            "close to your phone, then try again.",
        action = ErrorAction.RETRY
    )

    WizardFailure.WIFI_CREDENTIALS_REJECTED -> ErrorCopy(
        title = "The sensor could not join the network",
        message = "The sensor could not connect using this name and password. " +
            "Check them and try again. Nothing was saved, so the sensor is still " +
            "waiting for setup.",
        action = ErrorAction.RETRY
    )

    WizardFailure.LOCATION_UNAVAILABLE -> ErrorCopy(
        title = "Could not get your location",
        message = "QuakeAlert could not read your position just now. Check that " +
            "location is allowed for QuakeAlert, or place the pin on the map yourself.",
        action = ErrorAction.RETRY
    )

    WizardFailure.PLACE_NAME_MISSING -> ErrorCopy(
        title = "This place has no name yet",
        message = "No place name was found for this pin. Tap the place name above the " +
            "map and type one.",
        action = ErrorAction.NONE
    )

    WizardFailure.SENSOR_NEVER_CHECKED_IN -> ErrorCopy(
        title = "The sensor has not checked in",
        message = "Your sensor has not reported to the alert network yet. It keeps " +
            "trying on its own, so leave it powered on and check the Sensors list later.",
        action = ErrorAction.NONE
    )
}

/** One line under the place name field. */
internal fun DetailsError.message(): String = when (this) {
    DetailsError.NAME_REQUIRED -> "Enter a place name for this sensor."
    DetailsError.NAME_TOO_LONG ->
        "That place name is too long. Keep it under ${SensorNameRules.MAX_LENGTH} characters."
    DetailsError.NAME_HAS_NODE_ID ->
        "Use a place name here. The sensor's own ID is added for you."
    DetailsError.NAME_NOT_PLACE_LIKE -> "That does not look like a place name."
    DetailsError.POSITION_MISSING -> "Drag the map to place your sensor first."
}

/** One line under the Wi-Fi rows. */
internal fun LinkError.message(): String = when (this) {
    LinkError.SSID_REQUIRED -> "Choose the network your sensor should join."
    LinkError.PASSWORD_TOO_SHORT ->
        "That password is longer than the sensor can store. " +
            "Keep it under ${WifiRules.MAX_PASSWORD_LENGTH} characters."
}
