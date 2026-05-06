locals {
  default_labels = merge(var.labels, {
    environment = var.environment
    managed_by  = "terraform"
  })


  profiles = {
    "360p" = {
      width       = 640
      height      = 360
      bitrate_bps = 1000000 # 1 Mbps
      frame_rate  = 30
      gop_seconds = 2
      preset      = "slow"
      profile     = "high"
    }
    "480p" = {
      width       = 854
      height      = 450
      bitrate_bps = 2500000 # 2.5 Mbps
      frame_rate  = 30
      gop_seconds = 2
      preset      = "slow"
      profile     = "high"
    }
    "720p" = {
      width       = 1280
      height      = 720
      bitrate_bps = 5000000 # 5 Mbps
      frame_rate  = 30
      gop_seconds = 2
      preset      = "slow"
      profile     = "high"
    }
    "1080p" = {
      width       = 1920
      height      = 1080
      bitrate_bps = 8000000 # 8 Mbps
      frame_rate  = 30
      gop_seconds = 2
      preset      = "slow"
      profile     = "high"
    }
    "4k" = {
      width       = 3840
      height      = 2160
      bitrate_bps = 35000000 # 35 Mbps
      frame_rate  = 30
      gop_seconds = 2
      preset      = "slow"
      profile     = "high"
    }
    "720p-60fps" = {
      width       = 1280
      height      = 720
      bitrate_bps = 7500000 # 7.5 Mbps
      frame_rate  = 60
      gop_seconds = 2
      preset      = "slow"
      profile     = "high"
    }
    "1080p-60fps" = {
      width       = 1920
      height      = 1080
      bitrate_bps = 12000000 # 12 Mbps
      frame_rate  = 60
      gop_seconds = 2
      preset      = "slow"
      profile     = "high"
    }
    "4k-60fps" = {
      width       = 3840
      height      = 2160
      bitrate_bps = 53000000 # 53 Mbps
      frame_rate  = 60
      gop_seconds = 2
      preset      = "slow"
      profile     = "high"
    }
    "1080p-hdr" = {
      width       = 1920
      height      = 1080
      bitrate_bps = 10000000 # 10 Mbps
      frame_rate  = 30
      gop_seconds = 2
      preset      = "slow"
      profile     = "high"
    }
    "4k-hdr" = {
      width       = 3840
      height      = 2160
      bitrate_bps = 44000000 # 44 Mbps
      frame_rate  = 30
      gop_seconds = 2
      preset      = "slow"
      profile     = "high"
    }
  }

  region_profile_pairs = flatten([
    for region in var.regions : [
      for profile_name, profile in local.profiles : {
        key          = "${region.name}-${profile_name}"
        region       = region.name
        profile_name = profile_name
        profile      = profile
      }
    ]
  ])

  region_profile_map = {
    for pair in local.region_profile_pairs : pair.key => pair
  }
}


resource "google_transcoder_job_template" "profiles" {
  for_each = local.region_profile_map

  project         = var.project_id
  location        = each.value.region
  job_template_id = "${var.environment}-transcoder-${each.value.profile_name}"

  config {
    elementary_streams {
      key = "video-${each.value.profile_name}"
      video_stream {
        h264 {
          width_pixels  = each.value.profile.width
          height_pixels = each.value.profile.height
          bitrate_bps   = each.value.profile.bitrate_bps
          frame_rate    = each.value.profile.frame_rate
          gop_duration  = "${each.value.profile.gop_seconds}s"
          preset        = each.value.profile.preset
          profile       = each.value.profile.profile
        }
      }
    }

    elementary_streams {
      key = "audio-acc"

      audio_stream {
        codec             = "aac"
        bitrate_bps       = 128000
        channel_count     = 2
        sample_rate_hertz = 48000
      }
    }

    mux_streams {
      key                = "video-fmp4"
      container          = "fmp4"
      elementary_streams = ["video-${each.value.profile_name}"]

      segment_settings {
        segment_duration = "6s"
      }
    }

    mux_streams {
      key                = "audio-fmp4"
      container          = "fmp4"
      elementary_streams = ["audio-acc"]

      segment_settings {
        segment_duration = "6s"
      }
    }

    manifests {
      file_name   = "manifest.mpd"
      type        = "DASH"
      mux_streams = ["video-fmp4", "audio-fmp4"]
    }

    manifests {
      file_name   = "manifest.m3u8"
      type        = "HLS"
      mux_streams = ["video-fmp4", "audio-fmp4"]
    }

    output {
      uri = "gs://${var.transcoded_bucket_name}/"
    }

    pubsub_destination {
      topic = "projects/${var.project_id}/topics/${var.pubsub_topic_name}"
    }
  }

  labels = merge(local.default_labels, {
    profile = each.value.profile_name
    region  = each.value.region
  })
}
